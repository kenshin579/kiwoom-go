package wstransport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// 이 파일의 테스트는 전부 **Connect 를 부르지 않는다.** 그것이 요점이다.
//
// 3단계까지 Conn.Connect 를 부르는 곳은 이 패키지의 테스트뿐이었다. 사용자는 부를 수도
// 없다(Conn 은 internal/ 안에 산다). 그래서 생성된 실시간·조건검색 메서드 29개가 전부
// errNotConnected 로 떨어졌다 — 컴파일은 되는데 하나도 동작하지 않았다.

// 첫 구독이 스스로 붙어 REG 가 서버에 닿는지 본다. 이 수정의 핵심이다.
func TestSubscribe_Connect_없이_붙어서_REG_가_서버에_닿는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4); err != nil {
		t.Fatalf("Subscribe: %v — Connect 없이도 붙어야 한다", err)
	}

	// 로그인이 먼저, 그 다음 REG 여야 한다.
	login := waitPacket(t, f, "LOGIN")
	if login["token"] != "TKN" {
		t.Errorf("LOGIN token = %v, 기대 TKN", login["token"])
	}
	reg := waitPacket(t, f, "REG")
	data, _ := reg["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("REG data 길이 = %d, 기대 1", len(data))
	}
	first, _ := data[0].(map[string]any)
	items, _ := first["item"].([]any)
	if len(items) != 1 || items[0] != "005930" {
		t.Errorf("REG item = %v, 기대 [005930]", first["item"])
	}
}

// 조건검색(Request)도 같은 자리에서 붙어야 한다.
func TestRequest_Connect_없이_붙는다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = f.writeJSON(ws, map[string]any{"trnm": "CNSRLST", "return_code": 0})
		}()
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Request(ctx, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil); err != nil {
		t.Fatalf("Request: %v — Connect 없이도 붙어야 한다", err)
	}
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1", got)
	}
}

// 동시에 여러 구독이 시작해도 연결은 하나여야 한다.
//
// 서버 쪽 연결 수를 센다 — 클라이언트 쪽 상태를 보면 "다이얼은 둘인데 하나를 버렸다" 를
// 놓친다. 버려지는 소켓에 걸린 등록도 함께 사라지므로, 그것은 통과가 아니다.
func TestSubscribe_동시에_시작해도_연결은_하나다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = c.Subscribe(ctx, "0B", []string{fmt.Sprintf("%06d", i)}, 4)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("%d번 Subscribe: %v", i, err)
		}
	}
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1 — 동시 구독이 소켓을 여러 개 열었다", got)
	}
	if got := f.liveCount(); got != 1 {
		t.Errorf("살아 있는 연결 = %d, 기대 1", got)
	}
}

// 연결 실패는 호출자에게 그대로 와야 한다. 첫 구독이 자격증명 오류로 실패했다면
// 사용자가 그 자리에서 알아야 한다 — 조용히 삼키면 "왜 이벤트가 안 오지" 가 된다.
func TestSubscribe_연결_실패는_호출자에게_그대로_온다(t *testing.T) {
	f := newFakeServer(t)
	f.loginCode = 3 // 인증 거부

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4)
	if err == nil {
		t.Fatal("로그인이 거부됐는데 Subscribe 가 성공했다")
	}
	var le *LoginError
	if !errors.As(err, &le) {
		t.Fatalf("에러 타입 = %T(%v), 기대 *LoginError", err, err)
	}
	if ch != nil {
		t.Error("실패인데 채널을 줬다 — 소비자가 영영 막힌다")
	}
	// 실패한 구독이 재연결 목록에 남으면, 나중에 붙었을 때 구독자 없는 종목이 등록된다.
	c.subMu.Lock()
	regs, subs := len(c.regs), len(c.subs)
	c.subMu.Unlock()
	if regs != 0 || subs != 0 {
		t.Errorf("실패한 구독이 찌꺼기를 남겼다: regs=%d subs=%d", regs, subs)
	}
}

// Close 뒤에는 다시 붙지 않는다. 사용자가 끊은 줄 아는 소켓으로 트래픽이 다시 나가면 안 된다.
func TestSubscribe_Close_뒤에는_다시_붙지_않는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4); err != nil {
		t.Fatalf("첫 Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	_ = c.Close()

	if _, err := c.Subscribe(ctx, "0B", []string{"000660"}, 4); !errors.Is(err, errClosed) {
		t.Errorf("Close 뒤 Subscribe = %v, 기대 errClosed", err)
	}
	if err := c.Request(ctx, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil); !errors.Is(err, errClosed) {
		t.Errorf("Close 뒤 Request = %v, 기대 errClosed", err)
	}
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1 — Close 뒤에 다시 붙었다", got)
	}
}

// 재연결 루프가 도는 동안에는 게으른 연결이 새로 다이얼하지 않는다.
//
// 둘이 함께 다이얼하면 소켓이 둘 생기고, 곧바로 버려지는 쪽에 걸린 등록이 함께 사라진다.
// 깃발을 직접 세워 그 상태를 만든다 — 실제 재연결 타이밍을 노리면 불안정한 테스트가 된다.
func TestEnsureConnected_재연결_중에는_다이얼하지_않는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()

	c.mu.Lock()
	c.reconnecting = true // 재연결 루프가 도는 중
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4); !errors.Is(err, errReconnecting) {
		t.Errorf("Subscribe = %v, 기대 errReconnecting", err)
	}
	if got := f.connCount(); got != 0 {
		t.Errorf("연결 횟수 = %d, 기대 0 — 재연결 중에 따로 다이얼했다", got)
	}
}

// 재연결 루프가 끝나면 깃발이 내려가야 한다. 안 내려가면 그 뒤로 게으른 연결이
// 영원히 errReconnecting 만 돌려준다.
func TestReconnect_끝나면_깃발이_내려간다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		if n == 1 {
			go func() {
				time.Sleep(100 * time.Millisecond)
				_ = ws.CloseNow()
			}()
		}
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := c.Subscribe(ctx, "0B", []string{"005930"}, 8); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// 두 번째 연결이 생기고 깃발이 내려갈 때까지 기다린다.
	ok := waitFor(5*time.Second, func() bool {
		c.mu.Lock()
		flag := c.reconnecting
		c.mu.Unlock()
		return f.connCount() >= 2 && !flag
	})
	if !ok {
		c.mu.Lock()
		flag := c.reconnecting
		c.mu.Unlock()
		t.Fatalf("재연결 뒤에도 깃발이 서 있다 (연결 %d회, reconnecting=%v)", f.connCount(), flag)
	}
}

// **Critical 1 의 자리다.** Connect 없이 구독한 뒤 Close 하면 채널이 닫혀야 한다.
//
// 이 경로가 두 층 사이로 빠져 있었다. 조건검색 테스트는 c.Connect 를 **먼저** 부르고
// (그러면 c.life 가 이미 있다), 이 파일의 게으른 연결 테스트는 Close 를 보지 않는다.
// 그 사이에서 SubscribeCondition 은 — ensureConnected 를 부르지 않으므로 — 아직 연결이
// 없는 상태로 lifeCtx() 를 잡았고, 예전 lifeCtx 는 그때 context.Background() 를 줬다.
// 영영 끝나지 않는 컨텍스트다. Close 가 c.life 를 끊어도 정리 고루틴은 깨어나지 않아
// 채널이 닫히지 않고, 소비자는 for range 에서 영원히 막힌다.
//
// 손으로 쓴 조건검색 두 파일이 정확히 이 순서로 부른다 — 첫 푸시를 잃지 않으려고
// 요청보다 **먼저** 구독한다. 즉 갓 만든 Client 의 첫 호출이 늘 이 경로다.
func TestSubscribeCondition_Connect_없이_구독하고_Close_하면_채널이_닫힌다(t *testing.T) {
	// 서버가 없어도 된다 — SubscribeCondition 은 다이얼하지 않는다. 그것이 요점이다.
	c := New("ws://never", "", &stubToken{token: "TKN"})

	sub, err := c.SubscribeCondition(context.Background(), "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}
	_ = c.Close()

	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("닫히는 대신 무언가가 왔다")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Connect 없이 구독한 채널이 Close 뒤에도 닫히지 않았다")
	}

	// 자리도 내려가야 한다. 남으면 condSubs 가 영원히 줄지 않아, routeReal 이 실시간
	// 푸시마다 841 을 꺼내는 일을 영영 계속한다.
	ok := waitFor(3*time.Second, func() bool {
		c.subMu.Lock()
		defer c.subMu.Unlock()
		return len(c.subs) == 0 && c.condSubs == 0
	})
	if !ok {
		c.subMu.Lock()
		subs, cond := len(c.subs), c.condSubs
		c.subMu.Unlock()
		t.Fatalf("구독 자리가 남았다: subs=%d condSubs=%d", subs, cond)
	}
}

// 실시간 구독도 같은 규칙이다. 이쪽은 ensureConnected 를 부르므로 원래 안전했지만,
// lifeCtx 를 고치면서 반대로 깨지지 않았는지 함께 못 박는다.
func TestSubscribe_Connect_없이_구독하고_Close_하면_채널이_닫힌다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})

	sub, err := c.Subscribe(context.Background(), "0B", []string{"005930"}, 8)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	_ = c.Close()

	select {
	case _, ok := <-sub:
		if ok {
			for range sub {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close 뒤에도 채널이 닫히지 않았다")
	}
}

// Close 가 지나간 뒤에 붙은 구독도 채널이 닫혀야 한다.
//
// lifeCtx 가 수명을 게으르게 만들게 되면서 생긴 자리다 — 아무 생각 없이 만들면 그
// 수명은 이미 지나간 Close 를 놓쳐 영영 살아 있고, 채널은 영영 열린 채 남는다.
func TestSubscribeCondition_Close_뒤에_구독해도_채널이_닫힌다(t *testing.T) {
	c := New("ws://never", "", &stubToken{token: "TKN"})
	_ = c.Close()

	sub, err := c.SubscribeCondition(context.Background(), "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}
	select {
	case _, ok := <-sub:
		if ok {
			t.Fatal("닫히는 대신 무언가가 왔다")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close 뒤에 붙은 구독의 채널이 닫히지 않았다")
	}
}
