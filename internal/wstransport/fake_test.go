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

// fakeServer 는 키움 WS 서버 흉내다.
//
// 실서버 없이 로그인·PING·등록·재연결을 전부 검증하려면 이게 두꺼워야 한다.
// 모의투자가 실시간을 주는지 확인되지 않았으므로(설계 §11), 당분간 이것이 유일한 그물이다.
type fakeServer struct {
	*httptest.Server

	mu       sync.Mutex
	received []map[string]any // 서버가 받은 패킷 전부(로그인 포함)
	conns    int              // 몇 번 연결됐나 — 재연결 검증용

	// loginCode 는 로그인 응답의 return_code. 0 이 정상.
	loginCode int
	// onConn 은 연결이 열릴 때마다 불린다. 푸시를 밀어 넣거나 끊을 때 쓴다.
	onConn func(t *testing.T, n int, c *websocket.Conn)
	// beforeLoginAck 는 로그인 응답을 보내기 **전에** 불린다.
	// 규약을 어기는 서버(로그인 응답보다 다른 메시지가 먼저 오는 경우)를 흉내낼 때 쓴다.
	beforeLoginAck func(c *websocket.Conn)
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
		f.mu.Unlock()

		// 로그인 패킷을 받아 응답한다.
		var login map[string]any
		if err := f.readJSON(c, &login); err != nil {
			return
		}
		f.record(login)
		if f.beforeLoginAck != nil {
			f.beforeLoginAck(c)
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
			f.record(m)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// wsURL 은 httptest 의 http:// 를 ws:// 로 바꾼 것이다.
func (f *fakeServer) wsURL() string {
	return strings.Replace(f.URL, "http://", "ws://", 1)
}

func (f *fakeServer) record(m map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.received = append(f.received, m)
}

// packets 는 지금까지 받은 패킷을 복사해 돌려준다.
func (f *fakeServer) packets() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.received...)
}

func (f *fakeServer) connCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns
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
