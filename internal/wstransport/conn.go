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

	"github.com/kenshin579/kiwoom-go/internal/wire"
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

// errReconnecting 은 끊긴 뒤 다시 붙는 중이라 지금은 보낼 수 없다는 뜻이다.
//
// 여기서 새로 다이얼하지 않는다. 재연결 루프가 이미 그 일을 하고 있고, 둘이 함께
// 다이얼하면 소켓이 둘 생겨 하나가 곧바로 버려진다 — 그 버려지는 쪽에 걸린 등록도 함께
// 사라진다. 호출자에게 상태를 그대로 알리고 다시 시도하게 하는 편이 정직하다.
var errReconnecting = errors.New("kiwoom: websocket 이 끊겨 다시 붙는 중이다 — 잠시 뒤 다시 시도하라")

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
//
// ReturnCode 가 wire.Code 인 이유도 REST 와 같다: 서버가 엔드포인트에 따라 int 로도
// 숫자 문자열("0000"·"8005")로도 보낸다. int 로만 선언하면 문자열이 오는 순간
// readEnvelope 이 메시지 전체를 못 읽어 errBadMessage 로 버린다 — 로그인 거부도,
// 조건검색 업무 오류도 프레임 카운터만 올리고 사라진다.
type envelope struct {
	Trnm       string          `json:"trnm"`
	ReturnCode wire.Code       `json:"return_code"`
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

	// dialMu 는 다이얼을 한 줄로 세운다. c.mu 와 **따로** 둔다 — 다이얼은 초 단위로
	// 걸리는데 그동안 c.mu 를 들고 있으면 수신 루프도 Close 도 함께 멈춘다.
	dialMu sync.Mutex

	mu     sync.Mutex
	ws     *websocket.Conn
	done   chan struct{}      // 지금 도는 수신 루프의 종료 신호. Close 가 기다린다
	life   context.Context    // 연결의 수명. Connect 의 ctx 와 **다르다**
	stop   context.CancelFunc // Close 가 부른다
	closed bool               // Close 뒤에는 재연결하지 않는다
	// reconnecting 은 재연결 루프가 도는 중이라는 뜻이다.
	//
	// 게으른 연결(ensureConnected)이 그 루프와 부딪히지 않게 하는 깃발이다.
	// reconnectLoop 이 시작할 때 서고 끝날 때 내린다.
	reconnecting bool

	// subMu 는 구독 지도를 지킨다. 수신 고루틴과 호출자 고루틴이 함께 만진다.
	subMu sync.Mutex
	subs  map[subKey][]*sub
	regs  []*registration // 재연결 때 다시 보낼 등록. 해지되면 여기서도 빠진다
	// condSubs 는 살아 있는 조건검색 구독 수다(SubscribeCondition).
	//
	// 있고 없고만 본다 — 0 이면 routeReal 이 푸시마다 841 을 꺼내는 일을 건너뛴다.
	// regs 와 달리 재연결이 되살릴 수 없는 등록이라 여기에만 남는다.
	condSubs int

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

// ensureConnected 는 필요할 때 붙는다. 이미 붙어 있으면 아무것도 하지 않는다.
//
// **게으른 연결이 여기 있다.** 예전에는 "첫 구독 때 붙는다" 고 적어 놓고 그 코드가 없어,
// 생성된 실시간·조건검색 메서드가 전부 errNotConnected 로 떨어졌다. Connect 를 부르는
// 것은 테스트뿐이었다.
//
// sync.Once 를 쓰지 않는다. Once 는 (a) 재연결 뒤 다시 붙어야 하는 것도, (b) 첫 시도가
// 자격증명 오류로 실패했을 때 다음 호출자가 다시 시도하는 것도 못 한다 — 한 번 돌면
// 실패했어도 끝이라 그 뒤로는 영원히 "연결 안 됨" 이 된다.
//
// 대신 dialMu 로 다이얼을 한 줄로 세운다. 동시에 구독 열 개가 시작해도 첫 번째만
// 다이얼하고 나머지는 이미 붙은 소켓을 본다 — 연결은 하나다.
//
// 실패는 **그대로 돌려준다.** 첫 Subscribe 가 앱키 오류로 실패했다면 사용자가 그 자리에서
// 알아야 한다. 조용히 삼키고 나중에 다시 붙는 척하면, 아무 이벤트도 오지 않는 이유를
// 아무도 설명하지 못한다.
func (c *Conn) ensureConnected(ctx context.Context) error {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()

	c.mu.Lock()
	closed, ws, reconnecting := c.closed, c.ws, c.reconnecting
	c.mu.Unlock()

	switch {
	case closed:
		// Close 뒤에는 다시 붙지 않는다. 닫은 클라이언트가 조용히 되살아나면
		// 사용자가 끊은 줄 아는 소켓으로 트래픽이 다시 나간다.
		return errClosed
	case ws != nil:
		// 이미 소켓이 있다. 재연결 중이면 그 소켓은 죽어 있을 수 있지만, 그때는
		// 쓰기가 실패해 호출자에게 알려진다 — 여기서 새로 다이얼할 일은 아니다.
		return nil
	case reconnecting:
		return errReconnecting
	}
	return c.Connect(ctx)
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
		return &LoginError{ReturnCode: env.ReturnCode.Int(), ReturnMsg: env.ReturnMsg}
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

// lifeCtx 는 연결의 수명 컨텍스트다. 아직 없으면 **여기서 만든다.**
//
// 예전에는 없을 때 context.Background() 를 돌려줬다. 그것은 영영 끝나지 않는 컨텍스트라,
// 아직 다이얼하지 않은 연결에 붙은 정리 고루틴이 그것을 잡으면 Close 가 c.life 를 끊어도
// 깨어나지 않았다 — 채널이 닫히지 않아 for range 소비자가 영원히 막히고, 고루틴과
// 구독 자리가 샜다. SubscribeCondition 이 정확히 그 경로였다(ensureConnected 를 부르지
// 않고 수명만 잡는다). 손으로 쓴 조건검색 두 파일은 첫 푸시를 잃지 않으려고 요청보다
// **먼저** 구독하므로, 갓 만든 Client 의 첫 호출에서는 연결이 아직 없다.
//
// 게으르게 만드는 것으로 고친다. 여기서 다이얼하지는 않는다 — 그 결정은 그대로다.
// Connect 도 같은 `if c.life == nil` 가드를 쓰고, c.life 는 재연결에도 다시 만들지 않으므로
// 누가 먼저 만들든 결과는 하나다.
func (c *Conn) lifeCtx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.life == nil {
		c.life, c.stop = context.WithCancel(context.Background())
		if c.closed {
			// Close 가 이미 지나갔다. 방금 만든 수명은 그 Close 를 놓친 것이니 바로 끊는다 —
			// 그러지 않으면 닫힌 연결에 붙은 구독이 영영 닫히지 않는 채널을 준다.
			c.stop()
		}
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

	// closed 면 Close 가 끊은 것이다. c.ws != ws 면 이미 다른 소켓이 달렸다 —
	// 어느 쪽이든 재연결을 띄우면 루프가 둘이 된다.
	//
	// reconnecting 깃발은 루프를 띄우기로 **정한 그 락 안에서** 세운다. 락 밖에서
	// 세우면 그 틈에 ensureConnected 가 끼어들어 같이 다이얼한다.
	c.mu.Lock()
	start := !c.closed && c.ws == ws && life.Err() == nil
	if start {
		c.reconnecting = true
	}
	c.mu.Unlock()
	if !start {
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
