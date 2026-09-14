package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// 도메인. REST 와 달리 포트가 붙는다.
const (
	ProdURL = "wss://api.kiwoom.com:10000"
	MockURL = "wss://mockapi.kiwoom.com:10000"
)

// 경로. 시장마다 연결이 하나씩이다.
const (
	DomesticPath = "/api/dostk/websocket"
	OverseasPath = "/api/us/websocket"
)

// errClosed 는 이미 닫힌 연결에 무언가를 붙이려 했다는 뜻이다.
var errClosed = errors.New("kiwoom: websocket 이 이미 닫혔다")

// errBadMessage 는 메시지 **한 건**을 해석하지 못했다는 뜻이다. 연결은 멀쩡하다.
//
// 이것으로 재연결하지 않는다. WS 텍스트 프레임은 메시지 경계가 보장돼 한 건을 못 읽어도
// 뒤따르는 메시지가 어긋나지 않는다 — 재연결이 되찾는 것은 없고(잃은 프레임은 돌아오지
// 않는다) 대신 **진짜 구멍**을 만들어 전 구독자에게 GapError 를 뿌린다. 서버가 우리가
// 못 읽는 것을 계속 보내면(새 메시지 타입 등) 2초마다 재연결하는 핫 루프가 된다.
//
// 그렇다고 감추지도 않는다 — BadFrames 로 센다.
var errBadMessage = errors.New("kiwoom: websocket 메시지를 해석하지 못했다")

// TokenSource 는 접근토큰을 준다. internal/auth.Source 가 만족한다.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

// envelope 은 모든 WS 메시지의 공통 머리다.
//
// 업무 오류가 HTTP 200 + return_code 로 오는 REST 와 같은 구조다 — trnm 만으로는
// 성공·실패를 가를 수 없다.
type envelope struct {
	Trnm       string          `json:"trnm"`
	ReturnCode int             `json:"return_code"`
	ReturnMsg  string          `json:"return_msg"`
	Data       json.RawMessage `json:"data"`
	raw        []byte          // 원문. 조건검색 응답을 그대로 넘길 때 쓴다
}

// Conn 은 경로 하나에 대한 연결이다. 동시 호출에 안전하다.
type Conn struct {
	url   string
	path  string
	token TokenSource

	// retryDelay 는 재연결 간격이다. 0 이면 defaultRetryDelay. 테스트가 줄인다.
	//
	// Connect 전에 한 번만 정한다 — 붙은 뒤에 바꾸는 용도가 아니다.
	retryDelay time.Duration

	// connectTimeout 은 재연결 한 번에 주는 시간이다. 0 이면 connectAttemptTimeout.
	// retryDelay 와 같은 규칙으로 테스트가 줄인다.
	connectTimeout time.Duration

	mu     sync.Mutex
	ws     *websocket.Conn
	done   chan struct{}      // 지금 도는 수신 루프의 종료 신호. Close 가 기다린다
	life   context.Context    // 연결의 수명. Connect 의 ctx 와 **다르다**
	stop   context.CancelFunc // Close 가 부른다
	closed bool               // Close 뒤에는 재연결하지 않는다

	// subMu 는 구독 지도를 지킨다. 수신 고루틴과 호출자 고루틴이 함께 만진다.
	subMu sync.Mutex
	subs  map[subKey][]*sub
	regs  []*registration // 재연결 때 다시 보낼 등록. 해지되면 여기서도 빠진다

	// reqMu 는 요청 대기자를 지킨다. 짝짓기 열쇠가 trnm 뿐이라 trnm 당 한 건이다.
	reqMu sync.Mutex
	reqs  map[string]chan reqResult

	// badFrames 는 해석하지 못해 버린 메시지 수다. c.mu 가 지킨다.
	badFrames int
}

// BadFrames 는 해석하지 못해 버린 메시지 수다.
//
// 이런 프레임으로는 재연결하지 않지만(errBadMessage 참고) 세지도 않으면 그냥 감추는
// 것이 된다. 서버가 우리가 모르는 메시지를 보내기 시작하면 여기가 계속 는다.
func (c *Conn) BadFrames() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.badFrames
}

// New 는 연결기를 만든다. 아직 다이얼하지 않는다.
func New(url, path string, token TokenSource) *Conn {
	return &Conn{url: url, path: path, token: token}
}

// Connect 는 연결하고 로그인까지 마친다.
//
// 로그인이 인증 오류로 실패하면 토큰을 버리고 **로그인만** 한 번 다시 시도한다.
// 이것은 요청 재전송이 아니라 연결 수립이므로, 2단계의 "같은 요청을 다시 보내지 않는다"
// 규칙과 어긋나지 않는다 — 주문이 두 번 들어갈 여지가 없다.
func (c *Conn) Connect(ctx context.Context) error {
	// 연결의 수명은 Connect 에 준 ctx 와 다르다.
	//
	// ctx 는 "붙는 데까지" 의 시간 제한이다(예: 5초). 그것을 수신 루프에 그대로 물리면
	// 5초 뒤에 멀쩡한 연결이 죽는다. 수명은 Close 가 끊는다.
	c.mu.Lock()
	if c.life == nil {
		c.life, c.stop = context.WithCancel(context.Background())
	}
	c.mu.Unlock()

	return c.connectWithLoginRetry(ctx)
}

// connectWithLoginRetry 는 붙고 로그인한다. 로그인이 거부되면 토큰을 버리고 **로그인만**
// 한 번 다시 시도한다.
//
// Connect 와 재연결이 **같은 경로**를 타야 한다. 재연결도 연결 수립이다 — 여기를
// 건너뛰면 internal/auth.Source 가 만료 전까지 캐시한 토큰을 계속 내주므로, 서버에서
// 폐기된 토큰이면 죽은 자격증명으로 영원히 실패한다. 재연결은 아무도 지켜보지 않는
// 경로라 그 영구 실패가 조용히 이어진다.
func (c *Conn) connectWithLoginRetry(ctx context.Context) error {
	err := c.connectOnce(ctx)
	var le *LoginError
	if err == nil || !errors.As(err, &le) {
		return err
	}
	c.token.Invalidate()
	return c.connectOnce(ctx)
}

func (c *Conn) connectOnce(ctx context.Context) error {
	tok, err := c.token.Token(ctx)
	if err != nil {
		return err
	}

	ws, _, err := websocket.Dial(ctx, c.url+c.path, nil)
	if err != nil {
		return fmt.Errorf("kiwoom: websocket 연결 실패(%s): %w", c.path, err)
	}
	// 실시간 값 메시지는 크다(0D 는 163개 필드). 기본 32KiB 로는 모자랄 수 있다.
	ws.SetReadLimit(1 << 20)

	if err := writeJSON(ctx, ws, map[string]any{"trnm": "LOGIN", "token": tok}); err != nil {
		_ = ws.CloseNow()
		return err
	}

	// 로그인 응답을 **먼저** 받아야 한다. 다른 메시지가 먼저 오면 규약 위반이다.
	env, err := readEnvelope(ctx, ws)
	if err != nil {
		_ = ws.CloseNow()
		return err
	}
	if !strings.EqualFold(env.Trnm, "LOGIN") {
		_ = ws.CloseNow()
		return fmt.Errorf("kiwoom: 로그인 응답 전에 다른 메시지가 왔다: trnm=%q", env.Trnm)
	}
	if env.ReturnCode != 0 {
		_ = ws.CloseNow()
		return &LoginError{ReturnCode: env.ReturnCode, ReturnMsg: env.ReturnMsg}
	}

	c.mu.Lock()
	if c.closed {
		// 붙는 사이에 Close 가 왔다. 새 소켓을 달면 Close 가 놓친 것이 된다.
		c.mu.Unlock()
		_ = ws.CloseNow()
		return errClosed
	}
	old := c.ws
	c.ws = ws
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()

	// 이전 소켓을 **반드시** 닫는다.
	//
	// 덮어쓰기만 하면 끊길 때마다 연결이 샌다. 특히 읽기 실패의 원인이 깨진 메시지일
	// 때는 소켓 자체가 멀쩡해서, 닫지 않으면 서버 쪽 연결이 그대로 살아남는다.
	if old != nil {
		_ = old.CloseNow()
	}

	go c.readLoop(c.lifeCtx(), ws, done)
	return nil
}

// lifeCtx 는 연결의 수명 컨텍스트다.
func (c *Conn) lifeCtx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.life == nil {
		return context.Background()
	}
	return c.life
}

// readLoop 은 메시지를 받아 PING 을 걸러내고 나머지를 dispatch 한다.
//
// life 는 **연결의** 수명이지 Connect 에 준 ctx 가 아니다. 여기에 Connect 의 ctx 를
// 물리면 붙는 데 쓴 타임아웃이 지나는 순간 멀쩡한 연결이 죽는다.
//
// 해석하지 못한 메시지(errBadMessage)에서는 **계속 간다.** 재연결은 진짜 읽기 실패,
// 즉 소켓이 끝난 경우에만 한다.
func (c *Conn) readLoop(life context.Context, ws *websocket.Conn, done chan struct{}) {
	defer close(done)
	for {
		env, err := readEnvelope(life, ws)
		if err != nil {
			if errors.Is(err, errBadMessage) {
				// 해석 못 한 프레임 하나가 전 구독자에게 "구멍이 났다" 고 말할 근거는
				// 아니다. 다음 프레임은 멀쩡히 읽힌다 — 세어 두기만 한다.
				c.mu.Lock()
				c.badFrames++
				c.mu.Unlock()
				continue
			}
			c.onReadFailure(life, ws)
			return
		}
		// PING 은 받은 것을 그대로 되돌려 보내고 위로 올리지 않는다.
		if strings.EqualFold(env.Trnm, "PING") {
			ctx, cancel := context.WithTimeout(life, 5*time.Second)
			_ = write(ctx, ws, env.raw)
			cancel()
			continue
		}
		c.dispatch(env)
	}
}

// onReadFailure 는 읽기가 실패했을 때 — 즉 이 연결이 끝났을 때 — 를 다룬다.
//
// 두 가지를 한다. 대기 중인 요청을 깨우고, 재연결을 띄운다.
func (c *Conn) onReadFailure(life context.Context, ws *websocket.Conn) {
	// 조건검색 대기자는 **재전송하지 않는다.** 끊겼다고 알려 주고 호출자가 정하게
	// 한다 — 그러지 않으면 ctx 가 만료될 때까지 아무것도 모른 채 기다린다.
	//
	// 세대(ws == c.ws)를 보지 않고 부른다. 이미 다른 소켓이 달린 뒤에 지난 세대의 읽기
	// 루프가 여기 닿으면 새 연결의 대기자까지 깨우게 된다 — 보수적으로 그렇게 둔다.
	// 자세한 이유는 failRequests 의 주석에 있다.
	c.failRequests(errDisconnected)

	c.mu.Lock()
	closed := c.closed
	current := c.ws == ws
	c.mu.Unlock()
	// closed 면 Close 가 끊은 것이다. current 가 아니면 이미 다른 소켓이 달렸다 —
	// 어느 쪽이든 재연결을 띄우면 루프가 둘이 된다.
	if closed || !current || life.Err() != nil {
		return
	}
	go c.reconnectLoop(life, time.Now())
}

// dispatch 는 PING 이 아닌 메시지를 요청 대기자 또는 구독자에게 보낸다.
func (c *Conn) dispatch(env envelope) {
	if strings.EqualFold(env.Trnm, "REAL") {
		c.routeReal(env)
		return
	}
	// 요청 응답이 아니면 버린다. REG/REMOVE 의 확인 응답이 여기로 온다 —
	// 보낼 곳이 없으므로 버리는 것이 맞다.
	_ = c.routeResponse(env)
}

// Close 는 연결을 닫는다. 두 번 불러도 안전하다.
//
// 수신 루프가 **실제로 끝난 뒤에** 반환한다. closed 를 c.mu 아래에서 세우므로,
// 이 뒤로는 connectOnce 가 새 소켓을 달지 못한다 — 기다릴 대상이 하나로 고정된다.
func (c *Conn) Close() error {
	c.mu.Lock()
	c.closed = true
	stop := c.stop
	ws := c.ws
	done := c.done
	c.ws = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	var err error
	if ws != nil {
		err = ws.CloseNow()
	}
	if done != nil {
		<-done
	}
	return err
}

func readEnvelope(ctx context.Context, ws *websocket.Conn) (envelope, error) {
	_, b, err := ws.Read(ctx)
	if err != nil {
		return envelope{}, err
	}
	// 서버가 따옴표 없는 "PING" 문자열을 보내는 경우가 있다(공식 클라이언트도 이 경우를 본다).
	if s := strings.TrimSpace(string(b)); strings.EqualFold(s, "PING") {
		return envelope{Trnm: "PING", raw: b}, nil
	}
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		// errBadMessage 로 감싼다 — 호출자가 "이 메시지 하나를 못 읽었다" 와 "소켓이
		// 끝났다" 를 가릴 수 있어야 한다. 둘을 섞으면 깨진 한 건이 재연결을 부른다.
		return envelope{}, fmt.Errorf("kiwoom: websocket 메시지 파싱 실패: %w: %w", errBadMessage, err)
	}
	env.raw = b
	return env, nil
}

func writeJSON(ctx context.Context, ws *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return write(ctx, ws, b)
}

func write(ctx context.Context, ws *websocket.Conn, b []byte) error {
	return ws.Write(ctx, websocket.MessageText, b)
}
