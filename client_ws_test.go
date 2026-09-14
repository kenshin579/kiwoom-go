package kiwoom_test

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
	"github.com/kenshin579/kiwoom-go"
)

// 루트에서 실제로 붙는지, 그리고 붙은 뒤에 Close·BadFrames 가 닿는지를 한 번에 본다.
//
// **Connect 를 부르는 코드는 어디에도 없다.** 사용자는 부를 수 없다(연결은 internal/ 안에
// 산다). 그래서 게으른 연결이 없으면 이 테스트의 첫 줄부터 실패한다.
func TestClient_구독이_스스로_붙고_Close_와_BadFrames_가_닿는다(t *testing.T) {
	tokenSrv := newTokenServer(t)
	ws := newRealtimeServer(t)

	c, err := kiwoom.NewClient("AK", "SK",
		kiwoom.WithBaseURL(tokenSrv.URL),
		kiwoom.WithWSBaseURL(ws.url),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.DomesticRealtime.SubscribeDomesticStockTrade(ctx, "005930")
	if err != nil {
		t.Fatalf("SubscribeDomesticStockTrade: %v — 구독이 스스로 붙어야 한다", err)
	}

	select {
	case ev := <-ch:
		if ev.Err != nil {
			t.Fatalf("이벤트 Err = %v", ev.Err)
		}
		if ev.Symbol != "005930" || ev.Value.CurrentPrice != "-82000" {
			t.Errorf("이벤트 = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("실시간이 오지 않았다")
	}

	// 깨진 프레임은 재연결을 부르지 않는다. 대신 세어서 루트로 낸다 —
	// 감추면 "왜 어떤 이벤트는 안 오지" 를 아무도 설명하지 못한다.
	deadline := time.Now().Add(3 * time.Second)
	for c.BadFrames() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := c.BadFrames(); got != 1 {
		t.Errorf("BadFrames = %d, 기대 1 — 버린 프레임이 루트에서 보여야 한다", got)
	}

	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// 두 번 불러도 안전해야 한다. 연결을 하나도 안 연 미국 쪽도 함께 닫힌다.
	if err := c.Close(); err != nil {
		t.Errorf("두 번째 Close: %v", err)
	}
	// 서버 쪽 정리는 조금 늦는다(핸들러가 풀리는 시간). 기다렸다가 본다.
	closed := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if ws.liveCount() == 0 {
			closed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !closed {
		t.Errorf("Close 뒤에도 살아 있는 연결 = %d, 기대 0", ws.liveCount())
	}
}

// 소켓을 한 번도 열지 않은 클라이언트도 Close 할 수 있어야 한다.
// REST 만 쓴 사용자가 defer c.Close() 를 적었다고 터지면 안 된다.
func TestClient_붙은_적_없어도_Close_는_안전하다(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithMock())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if got := c.BadFrames(); got != 0 {
		t.Errorf("BadFrames = %d, 기대 0", got)
	}
}

func newTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"TKN","expires_dt":"20991231235959","return_code":0}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// realtimeServer 는 로그인에 답하고, REG 를 받으면 깨진 프레임 하나와 정상 REAL 하나를 보낸다.
type realtimeServer struct {
	url string

	mu   sync.Mutex
	live int
}

func (s *realtimeServer) liveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live
}

func newRealtimeServer(t *testing.T) *realtimeServer {
	t.Helper()
	s := &realtimeServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		s.mu.Lock()
		s.live++
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.live--
			s.mu.Unlock()
		}()

		write := func(b []byte) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return c.Write(ctx, websocket.MessageText, b)
		}
		writeJSON := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			return write(b)
		}
		read := func() (map[string]any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, b, err := c.Read(ctx)
			if err != nil {
				return nil, err
			}
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			return m, nil
		}

		if _, err := read(); err != nil { // LOGIN
			return
		}
		// return_code 를 **문자열**로 보낸다. 서버가 엔드포인트마다 형태를 바꾸는 것을
		// 루트까지 통과시키는지 여기서 함께 본다.
		if err := writeJSON(map[string]any{"trnm": "LOGIN", "return_code": "0000"}); err != nil {
			return
		}
		for {
			m, err := read()
			if err != nil {
				return
			}
			if m["trnm"] != "REG" {
				continue
			}
			// 깨진 프레임 하나 — 재연결이 아니라 BadFrames 로 세어야 한다.
			if err := write([]byte(`{"trnm":"REAL",`)); err != nil {
				return
			}
			if err := writeJSON(map[string]any{
				"trnm": "REAL",
				"data": []any{map[string]any{
					"type": "0B", "name": "주식체결", "item": "005930",
					"values": map[string]any{"10": "-82000"},
				}},
			}); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	s.url = strings.Replace(srv.URL, "http://", "ws://", 1)
	return s
}
