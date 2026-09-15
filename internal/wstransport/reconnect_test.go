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

// 결함 A — 새 소켓을 달면서 이전 것을 닫지 않으면 끊길 때마다 연결이 샌다.
//
// 소켓을 두 번 단다. 첫 소켓은 서버 쪽에서 멀쩡히 살아 있으므로, 클라이언트가 명시적으로
// 닫지 않으면 서버의 "살아 있는 연결 수" 가 2로 남는다.
func TestConnect_새_소켓을_달_때_이전_것을_닫는다(t *testing.T) {
	f := newFakeServer(t)

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()
	if !waitFor(3*time.Second, func() bool { return f.liveCount() == 1 }) {
		t.Fatalf("첫 연결 뒤 살아 있는 연결 = %d, 기대 1", f.liveCount())
	}

	if err := c.connectOnce(context.Background()); err != nil {
		t.Fatalf("connectOnce(두 번째): %v", err)
	}
	if got := f.connCount(); got != 2 {
		t.Fatalf("연결 횟수 = %d, 기대 2", got)
	}
	if !waitFor(3*time.Second, func() bool { return f.liveCount() == 1 }) {
		t.Fatalf("살아 있는 연결 = %d, 기대 1 — 새 소켓을 달면서 이전 것을 닫지 않는다", f.liveCount())
	}
}

// 깨진 JSON 한 건이 소켓을 갈아 끼우면 안 된다.
//
// WS 텍스트 프레임은 메시지 경계가 보장돼 한 건을 못 읽어도 뒤가 어긋나지 않는다.
// 재연결은 되찾는 것 없이 **진짜 구멍**만 만들고, 서버가 우리가 못 읽는 것을 계속
// 보내면 재연결 핫 루프가 된다. 그렇다고 감추지도 않는다 — BadFrames 로 센다.
func TestReadLoop_깨진_JSON_하나로는_재연결하지_않는다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 4)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	srv := recvConn(t, srvCh)

	for i := 0; i < 3; i++ {
		if err := f.writeRaw(srv, []byte(`{"trnm": 깨진`)); err != nil {
			t.Fatalf("깨진 프레임 전송: %v", err)
		}
	}
	// 같은 소켓으로 이어서 보낸 멀쩡한 메시지가 그대로 읽혀야 한다.
	pushReal(t, f, srv, "0B", "005930", "-82000")

	d := recvDelivery(t, sub)
	if d.Err != nil {
		t.Fatalf("깨진 프레임 뒤의 첫 이벤트가 에러다: %v — 소켓을 갈아 끼웠다", d.Err)
	}
	if d.Item != "005930" {
		t.Errorf("Item = %s, 기대 005930", d.Item)
	}

	time.Sleep(200 * time.Millisecond) // 뒤늦은 재연결이 있는지 본다
	if got := f.connCount(); got != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1 — 깨진 메시지 하나가 소켓을 갈아 끼웠다", got)
	}
	if got := c.BadFrames(); got != 3 {
		t.Errorf("BadFrames = %d, 기대 3 — 잃은 프레임을 세지 않고 감춘다", got)
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

// Critical 1 — 구멍이 "느림" 으로 둔갑하면 안 된다 (단위).
//
// 버퍼가 가득 찬 채로 끊기면 예전 구현은 dropped 를 올렸다. 그러면 사용자는
// *SlowConsumerError("느려서 버렸다") 만 보고 끊겼다는 사실은 영영 모른다 — 게다가 그
// 말은 사실도 아니다. 래치에 남았다가 자리가 나는 첫 순간에 와야 한다.
//
// -race 없이도 무는 테스트다. 고루틴을 쓰지 않고 순서를 직접 정한다.
func TestNotifyGap_자리가_없으면_래치에_남았다가_온다(t *testing.T) {
	c := &Conn{subs: map[subKey][]*sub{}}
	ch := make(chan Delivery, 1)
	s := &sub{key: subKey{"0B", "005930"}, ch: ch}
	c.subs[s.key] = append(c.subs[s.key], s)

	// 버퍼를 데이터로 채운다. 재연결 직후는 재등록 버스트로 이렇게 되기 쉽다 —
	// 구멍 알림이 가장 필요한 순간에 가장 잘 사라진다.
	ch <- Delivery{Type: "0B", Item: "005930"}

	since := time.Now().Add(-2 * time.Second)
	until := time.Now()
	if pending := c.notifyGap(since, until); pending != 1 {
		t.Fatalf("래치에 남은 구독 = %d, 기대 1 — 자리가 없는데 구멍을 버렸다", pending)
	}
	if s.dropped != 0 {
		t.Errorf("dropped = %d, 기대 0 — 구멍을 느림으로 셌다", s.dropped)
	}

	// 사용자가 먼저 보는 것은 밀려 있던 데이터다.
	if d := <-ch; d.Err != nil {
		t.Fatalf("첫 건이 에러다: %v", d.Err)
	}

	// 자리가 났다. 이제 구멍이 와야 한다.
	if pending := c.flushPendingGaps(); pending != 0 {
		t.Fatalf("자리가 났는데 %d개가 아직 래치에 남았다", pending)
	}
	select {
	case d := <-ch:
		var ge *GapError
		if !errors.As(d.Err, &ge) {
			var slow *SlowConsumerError
			if errors.As(d.Err, &slow) {
				t.Fatalf("구멍이 느림으로 둔갑했다: %v", d.Err)
			}
			t.Fatalf("기대 *GapError, 실제 %+v", d)
		}
		if !ge.Since.Equal(since) || !ge.Until.Equal(until) {
			t.Errorf("구간 = %v ~ %v, 기대 %v ~ %v", ge.Since, ge.Until, since, until)
		}
	default:
		t.Fatal("자리가 났는데 구멍이 오지 않았다")
	}
}

// 구멍 여럿이 겹치면 구간을 합친다 — 가장 이른 Since, 가장 늦은 Until.
//
// 겹친 둘을 따로 알릴 방법이 없다면 넓은 쪽으로 합치는 편이 정직하다. 좁게 잘라
// 말하면 사용자가 메꿔야 할 구간을 놓친다.
func TestNotifyGap_겹친_구멍은_구간이_합쳐진다(t *testing.T) {
	c := &Conn{subs: map[subKey][]*sub{}}
	ch := make(chan Delivery, 1)
	s := &sub{key: subKey{"0B", "005930"}, ch: ch}
	c.subs[s.key] = append(c.subs[s.key], s)
	ch <- Delivery{Type: "0B", Item: "005930"} // 버퍼를 채운다

	base := time.Now()
	early, late := base.Add(-10*time.Second), base.Add(10*time.Second)

	c.notifyGap(base.Add(-5*time.Second), base.Add(5*time.Second))
	c.notifyGap(early, late) // 더 넓은 구간
	c.notifyGap(base.Add(-time.Second), base.Add(time.Second))

	<-ch // 자리를 낸다
	if pending := c.flushPendingGaps(); pending != 0 {
		t.Fatalf("아직 %d개가 래치에 남았다", pending)
	}

	d := recvNow(t, ch)
	var ge *GapError
	if !errors.As(d.Err, &ge) {
		t.Fatalf("기대 *GapError, 실제 %+v", d)
	}
	if !ge.Since.Equal(early) {
		t.Errorf("Since = %v, 기대 %v (가장 이른 것)", ge.Since, early)
	}
	if !ge.Until.Equal(late) {
		t.Errorf("Until = %v, 기대 %v (가장 늦은 것)", ge.Until, late)
	}
	select {
	case d := <-ch:
		t.Errorf("구멍이 두 번 왔다: %+v", d)
	default:
	}
}

// Critical 1 — 끝에서 끝까지. 버퍼를 채운 채 끊었다 붙으면 구멍이 **결국** 온다.
//
// 검토자의 재현: [0] 데이터, [1] SlowConsumerError, 그리고 끝. 사용자는 끊긴 줄을
// 영영 몰랐다. 이제는 구멍이 와야 한다.
func TestReconnect_버퍼가_가득_차도_구멍은_결국_온다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 4)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 1) // 버퍼 1
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	srv := recvConn(t, srvCh)

	// 버퍼를 채우고, 실제로 찬 것을 확인한 뒤에 끊는다.
	pushReal(t, f, srv, "0B", "005930", "-82000")
	if !waitFor(3*time.Second, func() bool { return len(sub) == 1 }) {
		t.Fatal("버퍼가 차지 않았다 — 전제가 성립하지 않는다")
	}
	_ = srv.CloseNow()

	// 다시 붙어 등록까지 갔는지 본다. 여기서 notifyGap 이 불린다.
	if !waitFor(5*time.Second, func() bool { return len(regItems(f.packetsOn(2))) > 0 }) {
		t.Fatalf("다시 붙지 못했다 (연결 %d회)", f.connCount())
	}
	time.Sleep(100 * time.Millisecond) // 구멍이 래치에 얹힐 시간

	first := recvDelivery(t, sub)
	if first.Err != nil {
		t.Fatalf("버퍼에 있던 첫 건이 에러다: %v", first.Err)
	}

	// 자리가 났다. 구멍이 와야 한다 — 느림 경고가 아니라.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d := recvDelivery(t, sub)
		var ge *GapError
		if errors.As(d.Err, &ge) {
			return // 통과
		}
		var slow *SlowConsumerError
		if errors.As(d.Err, &slow) {
			t.Fatalf("구멍이 느림으로 둔갑했다: %v — 사용자는 끊긴 줄을 모른다", d.Err)
		}
	}
	t.Fatal("구멍이 끝내 오지 않았다")
}

// Critical 2 — 재연결이 계속 실패하는 동안 조용하면 안 된다.
//
// GapError 는 **결국 붙었을 때만** 온다. 장애가 이어지는 동안 아무 말도 없으면
// 구독자는 "장이 조용하다" 와 "한 시간째 못 붙고 있다" 를 구분할 수 없다.
func TestReconnect_계속_실패하면_되풀이해_알린다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 4)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	c.connectTimeout = 500 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 16)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	srv := recvConn(t, srvCh)

	f.setRefuse(true) // 서버는 살아 있지만 붙여 주지 않는다
	_ = srv.CloseNow()

	var got []*ReconnectingError
	deadline := time.Now().Add(5 * time.Second)
	for len(got) < 2 && time.Now().Before(deadline) {
		select {
		case d := <-sub:
			var re *ReconnectingError
			if errors.As(d.Err, &re) {
				got = append(got, re)
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if len(got) < 2 {
		t.Fatalf("*ReconnectingError 가 %d번 왔다, 기대 2회 이상 — 계속 실패하는 동안 조용하다", len(got))
	}
	if got[1].Attempts <= got[0].Attempts {
		t.Errorf("Attempts = %d → %d, 늘어야 한다", got[0].Attempts, got[1].Attempts)
	}
	if got[0].Last == nil {
		t.Error("Last 가 비어 있다 — 왜 실패했는지 알 수 없다")
	}
	if got[0].Since.IsZero() {
		t.Error("Since 가 비어 있다 — 언제부터 끊겼는지 알 수 없다")
	}
	if got[0].Error() == "" {
		t.Error("Error() 가 비어 있다")
	}

	// 서버가 돌아오면 붙고, 그제서야 구멍이 온다.
	f.setRefuse(false)
	waitGap(t, sub, 5*time.Second)
}

// Critical 3 — 재연결 경로에도 로그인 재시도가 있어야 한다.
//
// internal/auth.Source 는 만료가 가까워질 때까지 캐시한 토큰을 계속 준다. 서버가 그
// 토큰을 폐기했다면, 버리고 다시 받지 않는 한 죽은 자격증명으로 영원히 실패한다.
// Critical 2 와 합치면 보이지 않는 영구 장애다.
func TestReconnect_로그인이_거부되면_토큰을_버리고_다시_시도한다(t *testing.T) {
	f := newFakeServer(t)
	f.setRequireToken("T1")
	srvCh := make(chan *websocket.Conn, 8)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	tok := &rotatingToken{tokens: []string{"T1", "T2"}}
	c := New(f.wsURL(), "", tok)
	c.retryDelay = 20 * time.Millisecond
	c.connectTimeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 16)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")
	srv := recvConn(t, srvCh)
	if got := tok.invalidateCount(); got != 0 {
		t.Fatalf("Invalidate 횟수 = %d, 기대 0 — 첫 로그인은 통과했어야 한다", got)
	}

	// 서버가 T1 을 폐기했다. 토큰 캐시는 그것을 모른다.
	f.setRequireToken("T2")
	_ = srv.CloseNow()

	// 다시 붙어야 한다 — 그러려면 재연결이 토큰을 버리고 다시 받아야 한다.
	waitGap(t, sub, 5*time.Second)
	if got := tok.invalidateCount(); got == 0 {
		t.Error("재연결 경로가 토큰을 버리지 않았다 — 죽은 자격증명으로 영원히 실패한다")
	}
	if items := regItems(f.packets()); !items["005930"] {
		t.Errorf("다시 붙은 뒤 재등록이 없다 (등록 종목 = %v)", keys(items))
	}
}

// 재연결 시도 하나가 물려도 재연결 자체가 멈추면 안 된다.
//
// 수명 ctx 는 Close 전에 끝나지 않으므로, 시도마다 시간 제한이 없으면 로그인 응답에
// 한 번 물리는 순간 영영 멈춘다 — 그 침묵은 아무도 깨우지 못한다.
//
// **시간 예산을 넉넉히 잡는다.** 이 테스트가 재는 것은 "이어지는가" 지 "얼마나 빠른가"
// 가 아니다. 예전에는 3초였는데, 3단계에서 테스트가 늘어 전체 `-race` 실행이 5초에서
// 12초로 길어지자 그 예산을 넘겨 9회 중 2회 실패했다(패키지 단독으로는 늘 통과했다).
// 한 시도가 retryDelay(10ms) + connectTimeout(100ms) ≈ 110ms 이므로 4회는 0.5초면
// 끝난다 — 아래 예산은 그 스무 배다. 이만큼을 넘기면 느린 것이 아니라 멈춘 것이다.
func TestReconnect_시도_하나가_물려도_멈추지_않는다(t *testing.T) {
	const budget = 10 * time.Second

	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 8)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 10 * time.Millisecond
	c.connectTimeout = 100 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()
	srv := recvConn(t, srvCh)

	f.setSilentFrom(2) // 앞으로의 연결은 로그인 응답을 주지 않는다
	_ = srv.CloseNow()

	if !waitFor(budget, func() bool { return f.connCount() >= 4 }) {
		t.Fatalf("연결 시도 = %d, 기대 4회 이상 — 물린 시도에서 재연결이 멈췄다", f.connCount())
	}

	// 서버가 다시 답하면 붙는다.
	f.setSilentFrom(0)
	before := f.connCount()
	if !waitFor(budget, func() bool { return f.connCount() > before }) {
		t.Fatalf("시도가 이어지지 않았다 (연결 %d회)", f.connCount())
	}
}

// 재연결 뒤에도 PING 왕복이 살아 있어야 한다. 새 소켓에 수신 루프가 제대로 붙었는지는
// 이것으로만 확인된다 — 등록만 다시 보내고 루프가 죽어 있으면 조용히 아무것도 안 온다.
func TestReconnect_붙은_뒤에도_PING_에_답한다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 4)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	srv1 := recvConn(t, srvCh)
	_ = srv1.CloseNow()

	srv2 := recvConn(t, srvCh)
	if err := f.writeJSON(srv2, map[string]any{"trnm": "PING", "nonce": "after"}); err != nil {
		t.Fatalf("PING 전송: %v", err)
	}
	ok := waitFor(5*time.Second, func() bool {
		for _, p := range f.packetsOn(2) {
			if p["trnm"] == "PING" && p["nonce"] == "after" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Fatal("재연결 뒤 PING echo 가 오지 않았다 — 새 소켓에 수신 루프가 붙지 않았다")
	}
}

// 세대 확인 — 지난 세대의 읽기 실패로 재연결을 띄우면 루프가 둘이 된다.
//
// 이 확인에 테스트가 없어 지워도 24개가 다 통과했다. 여기서 붙잡는다.
func TestOnReadFailure_지난_세대의_실패는_재연결하지_않는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	c.retryDelay = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	// 두 번째 소켓을 직접 단다. 첫 소켓은 여기서 닫히고 그 수신 루프가 뒤늦게 죽는다 —
	// 그 죽음은 "이미 다른 소켓이 달린" 지난 세대의 것이다.
	if err := c.connectOnce(context.Background()); err != nil {
		t.Fatalf("connectOnce(두 번째): %v", err)
	}
	if got := f.connCount(); got != 2 {
		t.Fatalf("연결 횟수 = %d, 기대 2", got)
	}

	time.Sleep(300 * time.Millisecond) // retryDelay 의 여러 배
	if got := f.connCount(); got != 2 {
		t.Errorf("연결 횟수 = %d, 기대 2 — 지난 세대의 읽기 실패가 재연결을 띄웠다", got)
	}
}

// recvNow 는 이미 채널에 들어 있어야 할 한 건을 받는다. 없으면 바로 실패다 —
// 막혀서 10분 뒤 패닉으로 죽는 것보다 지금 이유를 말하는 편이 낫다.
func recvNow(t *testing.T, ch <-chan Delivery) Delivery {
	t.Helper()
	select {
	case d, ok := <-ch:
		if !ok {
			t.Fatal("채널이 닫혔다")
		}
		return d
	default:
		t.Fatal("채널이 비어 있다 — 와야 할 것이 오지 않았다")
	}
	return Delivery{}
}

// recvConn 은 서버 쪽 연결 하나를 받는다. 안 오면 실패다.
func recvConn(t *testing.T, ch <-chan *websocket.Conn) *websocket.Conn {
	t.Helper()
	select {
	case ws := <-ch:
		return ws
	case <-time.After(5 * time.Second):
		t.Fatal("서버 쪽 연결이 오지 않았다")
	}
	return nil
}

// pushReal 은 REAL 한 건을 밀어 넣는다.
func pushReal(t *testing.T, f *fakeServer, ws *websocket.Conn, typ, item, price string) {
	t.Helper()
	if err := f.writeJSON(ws, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": typ, "name": "주식체결", "item": item,
			"values": map[string]any{"10": price},
		}},
	}); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}
}

// waitGap 은 *GapError 가 올 때까지 채널을 비우며 기다린다. 안 오면 실패다.
func waitGap(t *testing.T, ch <-chan Delivery, d time.Duration) *GapError {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case got, ok := <-ch:
			if !ok {
				t.Fatal("구멍을 기다리는데 채널이 닫혔다")
			}
			var ge *GapError
			if errors.As(got.Err, &ge) {
				return ge
			}
		case <-deadline:
			t.Fatal("*GapError 가 오지 않았다 — 다시 붙지 못했거나 구멍을 감췄다")
		}
	}
}
