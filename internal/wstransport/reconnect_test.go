package wstransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestReconnect_끊기면_다시_붙고_재구독한다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n == 1 {
			// 첫 연결은 REG 를 받은 뒤 끊는다.
			go func() {
				time.Sleep(200 * time.Millisecond)
				_ = ws.CloseNow()
			}()
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond // 테스트를 빠르게
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Subscribe(ctx, "0B", []string{"005930"}, 8); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")

	// 두 번째 연결이 생기고, 거기에도 REG 가 다시 가야 한다.
	ok := waitFor(5*time.Second, func() bool {
		regs := 0
		for _, p := range f.packets() {
			if p["trnm"] == "REG" {
				regs++
			}
		}
		return f.connCount() >= 2 && regs >= 2
	})
	if !ok {
		t.Fatalf("재연결/재구독이 없었다 (연결 %d회)", f.connCount())
	}
}

func TestReconnect_구멍을_채널로_알린다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n == 1 {
			go func() {
				time.Sleep(200 * time.Millisecond)
				_ = ws.CloseNow()
			}()
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 8)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	timeout := time.After(5 * time.Second)
	for {
		select {
		case d := <-sub:
			var ge *GapError
			if d.Err != nil && errors.As(d.Err, &ge) {
				if ge.Since.IsZero() || ge.Until.IsZero() {
					t.Errorf("GapError 의 시각이 비어 있다: %+v", ge)
				}
				if ge.Until.Before(ge.Since) {
					t.Errorf("Until 이 Since 보다 이르다: %+v", ge)
				}
				return // 통과
			}
		case <-timeout:
			t.Fatal("GapError 가 오지 않았다")
		}
	}
}

// 결함 A — 재연결이 이전 소켓을 닫지 않으면 끊길 때마다 연결이 샌다.
//
// 깨진 JSON 으로 끊는다. 소켓 자체는 멀쩡한데 수신 루프만 죽는 상황이라, 새 소켓을
// 달면서 이전 것을 명시적으로 닫지 않으면 이전 연결이 서버 쪽에 그대로 살아남는다.
// 서버가 세어 주는 "살아 있는 연결 수" 로 확인한다.
func TestReconnect_이전_소켓을_닫는다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n <= 3 {
			f.writeRaw(ws, []byte(`{"trnm": 깨진`))
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if !waitFor(5*time.Second, func() bool { return f.connCount() >= 4 }) {
		t.Fatalf("네 번째 연결까지 가지 못했다 (연결 %d회)", f.connCount())
	}
	if !waitFor(2*time.Second, func() bool { return f.liveCount() == 1 }) {
		t.Fatalf("살아 있는 연결 = %d, 기대 1 — 재연결이 이전 소켓을 닫지 않는다", f.liveCount())
	}
}

// 결함 B — 구독 해지와 재연결이 겹치면 닫힌 채널에 보내 panic 이다.
//
// closeSubs 가 "지도에서 빼기 + 채널 닫기" 를 한 락 안에서 하므로, notifyGap 도
// 같은 락 아래에서 closed 를 보고 보내야 한다. 락 밖에서 보내면 여기서 죽는다.
func TestNotifyGap_구독_해지와_겹쳐도_죽지_않는다(t *testing.T) {
	since := time.Now()
	until := since.Add(time.Second)

	for i := 0; i < 2000; i++ {
		c := &Conn{subs: map[subKey][]*sub{}}
		ch := make(chan Delivery, 1)
		s := &sub{key: subKey{"0B", "005930"}, ch: ch}
		c.subs[s.key] = append(c.subs[s.key], s)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			c.notifyGap(since, until)
		}()
		go func() {
			defer wg.Done()
			<-start
			c.closeSubs([]*sub{s}, ch)
		}()
		close(start)
		wg.Wait()
	}
}

// 결함 C — 해지된 등록이 regs 에 남으면 재연결이 그것까지 다시 보낸다.
//
// 구독자가 없는 종목을 계속 받게 되고, 서버 등록 한도도 축낸다.
func TestReconnect_해지된_등록은_다시_보내지_않는다(t *testing.T) {
	f := newFakeServer(t)
	drop := make(chan struct{})
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n == 1 {
			go func() {
				<-drop
				_ = ws.CloseNow()
			}()
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	goneCtx, cancelGone := context.WithCancel(context.Background())
	if _, err := c.Subscribe(goneCtx, "0B", []string{"005930"}, 8); err != nil {
		t.Fatalf("Subscribe(해지될 것): %v", err)
	}
	keepCtx, cancelKeep := context.WithCancel(context.Background())
	defer cancelKeep()
	if _, err := c.Subscribe(keepCtx, "0B", []string{"000660"}, 8); err != nil {
		t.Fatalf("Subscribe(남을 것): %v", err)
	}
	waitPacket(t, f, "REG")

	// REMOVE 가 서버에 닿았다면 regs 에서 빼는 것은 이미 끝난 뒤다(먼저 뺀다).
	cancelGone()
	waitPacket(t, f, "REMOVE")

	close(drop) // 이제 끊는다

	if !waitFor(5*time.Second, func() bool { return len(regItems(f.packetsOn(2))) > 0 }) {
		t.Fatalf("두 번째 연결에 REG 가 오지 않았다 (연결 %d회)", f.connCount())
	}
	// 뒤늦게 더 올 수도 있으니 잠시 더 본다.
	time.Sleep(200 * time.Millisecond)

	items := regItems(f.packetsOn(2))
	if _, bad := items["005930"]; bad {
		t.Errorf("재연결이 해지된 005930 을 다시 등록했다 (재등록 종목 = %v)", keys(items))
	}
	if _, ok := items["000660"]; !ok {
		t.Errorf("재연결이 살아 있는 000660 을 등록하지 않았다 (재등록 종목 = %v)", keys(items))
	}
}

// 결함 D — 끊겼는데 요청 대기자가 그대로 남으면 ctx 만료로만 풀린다.
//
// 조건검색은 재전송하지 않으므로, 호출자가 빨리 에러를 받아 스스로 정하는 편이 낫다.
func TestRequest_끊기면_대기자가_에러를_받는다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n == 1 {
			go func() {
				time.Sleep(150 * time.Millisecond)
				_ = ws.CloseNow()
			}()
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := c.Request(ctx, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil)
	if !errors.Is(err, errDisconnected) {
		t.Fatalf("Request = %v, 기대 errDisconnected", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("%v 만에 풀렸다 — ctx 만료로만 풀린 것으로 보인다", elapsed)
	}
}

// 결함 E — done 채널을 만들어 두고 아무도 기다리지 않으면 죽은 코드다.
//
// Close 가 반환한 뒤에도 수신 루프가 돌고 있으면, 닫았다는 말이 거짓이 된다.
func TestClose_수신_루프_종료를_기다린다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		t.Fatal("연결됐는데 done 이 없다")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Close 가 반환했는데 수신 루프가 아직 돈다")
	}
}

// 결함 F-1 — 서버가 로그인 응답 없이 침묵하면 ctx 로 풀려야 한다.
func TestConnect_로그인_응답이_없으면_ctx_로_풀린다(t *testing.T) {
	f := newFakeServer(t)
	f.silentLogin = true

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.Connect(ctx)
	if err == nil {
		_ = c.Close()
		t.Fatal("서버가 침묵하는데 Connect 가 성공했다")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("%v 만에 풀렸다 — ctx 가 먹히지 않는다", elapsed)
	}
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1 — 로그인 오류가 아니면 다시 시도하지 않는다", got)
	}
}

// 결함 F-2 — 깨진 JSON 은 readEnvelope 이 에러로 내야 한다. 조용히 빈 봉투를
// 돌려주면 trnm 없는 메시지가 dispatch 를 타고 들어간다.
func TestReadEnvelope_깨진_JSON_은_에러다(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.CloseNow() }()
		_ = ws.Write(r.Context(), websocket.MessageText, []byte(`{"trnm": 깨진`))
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, strings.Replace(srv.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = ws.CloseNow() }()

	env, err := readEnvelope(ctx, ws)
	if err == nil {
		t.Fatalf("깨진 JSON 인데 에러가 없다: %+v", env)
	}
	if !strings.Contains(err.Error(), "파싱 실패") {
		t.Errorf("에러 = %v, 기대: 파싱 실패", err)
	}
}

// 결함 F-3 — 서버가 로그인 패킷을 읽지도 않고 즉시 끊으면 에러여야 한다.
func TestConnect_서버가_즉시_끊으면_에러다(t *testing.T) {
	f := newFakeServer(t)
	f.closeOnAccept = true

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		_ = c.Close()
		t.Fatal("서버가 즉시 끊었는데 Connect 가 성공했다")
	}
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1 — 로그인 오류가 아니면 다시 시도하지 않는다", got)
	}
}

// waitFor 는 조건이 참이 될 때까지 기다린다. 시간 안에 못 보면 false.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// regItems 는 REG 패킷들이 등록한 종목코드를 모은다.
func regItems(ps []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, p := range ps {
		if p["trnm"] != "REG" {
			continue
		}
		data, _ := p["data"].([]any)
		for _, d := range data {
			m, _ := d.(map[string]any)
			items, _ := m["item"].([]any)
			for _, it := range items {
				if s, ok := it.(string); ok {
					out[s] = true
				}
			}
		}
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
