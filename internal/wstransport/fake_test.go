package wstransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// packet 은 서버가 받은 패킷 하나다. 몇 번째 연결에서 받았는지 함께 들고 있다 —
// 재연결 뒤에 무엇이 다시 왔는지 가르려면 연결 번호가 있어야 한다.
type packet struct {
	conn int
	m    map[string]any
}

// fakeServer 는 키움 WS 서버 흉내다.
//
// 실서버 없이 로그인·PING·등록·재연결을 전부 검증하려면 이게 두꺼워야 한다.
// 모의투자가 실시간을 주는지 확인되지 않았으므로(설계 §11), 당분간 이것이 유일한 그물이다.
type fakeServer struct {
	*httptest.Server

	mu       sync.Mutex
	received []packet // 서버가 받은 패킷 전부(로그인 포함)
	conns    int      // 몇 번 연결됐나 — 재연결 검증용
	live     int      // 지금 살아 있는 연결 수 — 소켓 누수 검증용

	// loginCode 는 로그인 응답의 return_code. 0 이 정상.
	loginCode int
	// onConn 은 연결이 열릴 때마다 불린다. 푸시를 밀어 넣거나 끊을 때 쓴다.
	onConn func(t *testing.T, n int, c *websocket.Conn)
	// beforeLoginAck 는 로그인 응답을 보내기 **전에** 불린다.
	// 규약을 어기는 서버(로그인 응답보다 다른 메시지가 먼저 오는 경우)를 흉내낼 때 쓴다.
	beforeLoginAck func(c *websocket.Conn)
	// silentLogin 이면 로그인 응답을 아예 보내지 않는다. 끊지도 않는다 —
	// 클라이언트가 ctx 로 풀리는지 보는 용도라 소켓은 살려 둬야 한다.
	silentLogin bool
	// closeOnAccept 면 로그인 패킷을 읽지도 않고 바로 끊는다.
	closeOnAccept bool
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		f.mu.Lock()
		f.conns++
		n := f.conns
		f.live++
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.live--
			f.mu.Unlock()
		}()

		if f.closeOnAccept {
			return
		}

		// 로그인 패킷을 받아 응답한다.
		var login map[string]any
		if err := f.readJSON(c, &login); err != nil {
			return
		}
		f.record(n, login)
		if f.beforeLoginAck != nil {
			f.beforeLoginAck(c)
		}
		if f.silentLogin {
			<-r.Context().Done() // 응답하지 않고, 끊지도 않는다
			return
		}
		_ = f.writeJSON(c, map[string]any{
			"trnm": "LOGIN", "return_code": f.loginCode, "return_msg": "",
		})

		if f.onConn != nil {
			f.onConn(t, n, c)
		}

		// 남은 시간 동안 받은 패킷을 기록만 한다.
		for {
			var m map[string]any
			if err := f.readJSON(c, &m); err != nil {
				return
			}
			f.record(n, m)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// wsURL 은 httptest 의 http:// 를 ws:// 로 바꾼 것이다.
func (f *fakeServer) wsURL() string {
	return strings.Replace(f.URL, "http://", "ws://", 1)
}

func (f *fakeServer) record(n int, m map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.received = append(f.received, packet{conn: n, m: m})
}

// packets 는 지금까지 받은 패킷을 연결 구분 없이 돌려준다.
func (f *fakeServer) packets() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(f.received))
	for _, p := range f.received {
		out = append(out, p.m)
	}
	return out
}

// packetsOn 은 n 번째 연결에서 받은 패킷만 돌려준다.
func (f *fakeServer) packetsOn(n int) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, p := range f.received {
		if p.conn == n {
			out = append(out, p.m)
		}
	}
	return out
}

func (f *fakeServer) connCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
}

// liveCount 는 아직 닫히지 않은 연결 수다. 재연결이 이전 소켓을 흘리면 여기가 는다.
func (f *fakeServer) liveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live
}

func (f *fakeServer) readJSON(c *websocket.Conn, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (f *fakeServer) writeJSON(c *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return f.writeRaw(c, b)
}

// writeRaw 는 바이트를 그대로 보낸다. 깨진 JSON 을 흉내낼 때 쓴다.
func (f *fakeServer) writeRaw(c *websocket.Conn, b []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Write(ctx, websocket.MessageText, b)
}

// stubToken 은 고정 토큰을 주는 TokenSource 다.
type stubToken struct {
	mu          sync.Mutex
	token       string
	invalidated int
}

func (s *stubToken) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token, nil
}

func (s *stubToken) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidated++
}

func (s *stubToken) invalidateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invalidated
}
