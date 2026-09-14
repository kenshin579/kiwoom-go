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

	mu     sync.Mutex
	ws     *websocket.Conn
	done   chan struct{}      // 수신 루프 종료 신호
	life   context.Context    // 연결의 수명. Connect 의 ctx 와 **다르다**
	stop   context.CancelFunc // Close 가 부른다
	closed bool               // Close 뒤에는 재연결하지 않는다

	// subMu 는 구독 지도를 지킨다. 수신 고루틴과 호출자 고루틴이 함께 만진다.
	subMu sync.Mutex
	subs  map[subKey][]*sub
	regs  []registration // 재연결 때 다시 보낼 등록(Task 4 가 읽는다)

	// reqMu 는 요청 대기자를 지킨다. 짝짓기 열쇠가 trnm 뿐이라 trnm 당 한 건이다.
	reqMu sync.Mutex
	reqs  map[string]chan envelope
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

	if err := c.connectOnce(ctx); err != nil {
		var le *LoginError
		if !errors.As(err, &le) {
			return err
		}
		c.token.Invalidate()
		return c.connectOnce(ctx)
	}
	return nil
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
	c.ws = ws
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()

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
func (c *Conn) readLoop(life context.Context, ws *websocket.Conn, done chan struct{}) {
	defer close(done)
	for {
		env, err := readEnvelope(life, ws)
		if err != nil {
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
func (c *Conn) Close() error {
	c.mu.Lock()
	c.closed = true
	stop := c.stop
	ws := c.ws
	c.ws = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if ws == nil {
		return nil
	}
	return ws.CloseNow()
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
		return envelope{}, fmt.Errorf("kiwoom: websocket 메시지 파싱 실패: %w", err)
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
