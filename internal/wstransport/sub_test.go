package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSubscribe_REG_패킷을_배열로_보낸다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Subscribe(ctx, "0B", []string{"005930", "000660"}, 4); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	reg := waitPacket(t, f, "REG")
	data, _ := reg["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data 길이 = %d, 기대 1", len(data))
	}
	first, _ := data[0].(map[string]any)
	items, _ := first["item"].([]any)
	types, _ := first["type"].([]any)
	if len(items) != 2 || items[0] != "005930" {
		t.Errorf("item = %v, 기대 [005930 000660] (배열이어야 한다)", first["item"])
	}
	if len(types) != 1 || types[0] != "0B" {
		t.Errorf("type = %v, 기대 [0B] (배열이어야 한다)", first["type"])
	}
}

func TestSubscribe_푸시를_구독자에게_라우팅한다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = f.writeJSON(ws, map[string]any{
				"trnm": "REAL",
				"data": []any{map[string]any{
					"type": "0B", "name": "주식체결", "item": "005930",
					"values": map[string]any{"10": "-82000", "20": "093015"},
				}},
			})
		}()
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
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

	select {
	case d := <-sub:
		if d.Err != nil {
			t.Fatalf("Err = %v", d.Err)
		}
		if d.Item != "005930" || d.Type != "0B" {
			t.Errorf("Type/Item = %s/%s, 기대 0B/005930", d.Type, d.Item)
		}
		var vals map[string]string
		if err := json.Unmarshal(d.Values, &vals); err != nil {
			t.Fatalf("values 파싱: %v", err)
		}
		if vals["10"] != "-82000" {
			t.Errorf("values[10] = %q, 기대 -82000", vals["10"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("푸시가 오지 않았다")
	}
}

func TestSubscribe_ctx_취소가_REMOVE_를_보내고_채널을_닫는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { _ = c.Close() }()
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	sub, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitPacket(t, f, "REG")

	cancel()
	waitPacket(t, f, "REMOVE")

	select {
	case _, ok := <-sub:
		if ok {
			// 남은 이벤트를 다 비우면 닫힌다.
			for range sub {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}
}

func TestRequest_trnm_으로_응답을_짝짓는다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = f.writeJSON(ws, map[string]any{
				"trnm": "CNSRLST", "return_code": 0,
				"data": []any{map[string]any{"seq": "0", "name": "내조건"}},
			})
		}()
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	var out struct {
		Data []struct {
			Seq  string `json:"seq"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := c.Request(ctx, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, &out); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(out.Data) != 1 || out.Data[0].Name != "내조건" {
		t.Errorf("응답 = %+v, 기대 내조건 1건", out.Data)
	}
}

// waitPacket 은 서버가 그 trnm 패킷을 받을 때까지 기다린다.
func waitPacket(t *testing.T, f *fakeServer, trnm string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range f.packets() {
			if p["trnm"] == trnm {
				return p
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s 패킷이 오지 않았다", trnm)
	return nil
}

// 함정 1번(느린 소비자가 소켓 전체를 막는다)이 실제로 막혔는지 본다.
//
// 버퍼 1짜리 구독에 아무도 읽지 않는 채로 5건을 밀어 넣는다. deliver 가 막히는
// 구현이라면 수신 루프가 두 번째 건에서 멈추고, 뒤이어 보낸 PING 에 영영 답하지
// 못한다. PING echo 가 돌아온다는 것은 다섯 건을 전부 막히지 않고 흘려보냈다는 뜻이다.
func TestSubscribe_느린_소비자는_소켓을_막지_않는다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 1)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
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
	srv := <-srvCh

	push := func(price string) {
		t.Helper()
		if err := f.writeJSON(srv, map[string]any{
			"trnm": "REAL",
			"data": []any{map[string]any{
				"type": "0B", "name": "주식체결", "item": "005930",
				"values": map[string]any{"10": price},
			}},
		}); err != nil {
			t.Fatalf("푸시 전송: %v", err)
		}
	}

	for i := 0; i < 5; i++ {
		push(fmt.Sprintf("-%d", i))
	}

	// 소켓이 막혔다면 이 PING 은 영영 처리되지 않는다.
	if err := f.writeJSON(srv, map[string]any{"trnm": "PING", "nonce": "slow"}); err != nil {
		t.Fatalf("PING 전송: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	echoed := false
	for !echoed && time.Now().Before(deadline) {
		for _, p := range f.packets() {
			if p["trnm"] == "PING" && p["nonce"] == "slow" {
				echoed = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !echoed {
		t.Fatal("PING echo 가 오지 않았다 — 느린 소비자가 수신 루프를 막았다")
	}

	// 버퍼에는 첫 건만 남아 있다. 나머지 4건은 버려졌다.
	first := recvDelivery(t, sub)
	if first.Err != nil {
		t.Fatalf("첫 건이 에러다: %v", first.Err)
	}

	// 자리가 났으니 다음 푸시 때 버린 건수를 알려야 한다.
	push("-9")
	next := recvDelivery(t, sub)
	var slow *SlowConsumerError
	if !errors.As(next.Err, &slow) {
		t.Fatalf("기대: *SlowConsumerError, 실제: %+v", next)
	}
	if slow.Dropped != 4 {
		t.Errorf("Dropped = %d, 기대 4", slow.Dropped)
	}
	if slow.Type != "0B" || slow.Item != "005930" {
		t.Errorf("Type/Item = %s/%s, 기대 0B/005930", slow.Type, slow.Item)
	}
}

// 같은 종목을 두 번 구독하면 둘 다 받아야 한다. subs 가 슬라이스인 이유다.
func TestSubscribe_같은_종목을_두_번_구독하면_둘_다_받는다(t *testing.T) {
	f := newFakeServer(t)
	srvCh := make(chan *websocket.Conn, 1)
	f.onConn = func(t *testing.T, n int, ws *websocket.Conn) { srvCh <- ws }

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	a, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4)
	if err != nil {
		t.Fatalf("Subscribe a: %v", err)
	}
	b, err := c.Subscribe(ctx, "0B", []string{"005930"}, 4)
	if err != nil {
		t.Fatalf("Subscribe b: %v", err)
	}
	waitPacket(t, f, "REG")
	srv := <-srvCh

	if err := f.writeJSON(srv, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "0B", "name": "주식체결", "item": "005930",
			"values": map[string]any{"10": "-82000"},
		}},
	}); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	for i, sub := range []<-chan Delivery{a, b} {
		d := recvDelivery(t, sub)
		if d.Err != nil {
			t.Fatalf("%d번 구독 Err = %v", i, d.Err)
		}
		if d.Item != "005930" {
			t.Errorf("%d번 구독 Item = %s, 기대 005930", i, d.Item)
		}
	}
}

// 짝짓기 열쇠가 trnm 뿐이라, 같은 trnm 을 동시에 두 건 보내면 응답이 뒤바뀐다.
// 두 번째 요청은 보내기 전에 에러여야 한다.
func TestRequest_같은_trnm_을_동시에_두_건_보내면_에러다(t *testing.T) {
	f := newFakeServer(t) // 응답하지 않는 서버
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	ctx1, cancel1 := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- c.Request(ctx1, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil)
	}()
	// 서버가 첫 패킷을 받았으면 대기자 등록은 이미 끝난 뒤다(등록이 먼저다).
	waitPacket(t, f, "CNSRLST")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	err := c.Request(ctx2, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil)
	if err == nil {
		t.Fatal("두 번째 요청이 통과했다 — 응답이 뒤바뀔 수 있다")
	}
	if !strings.Contains(err.Error(), "이미 대기 중") {
		t.Errorf("에러 = %v, 기대: 이미 대기 중", err)
	}

	cancel1()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Errorf("첫 요청 = %v, 기대 context.Canceled", err)
	}
}

// recvDelivery 는 채널에서 한 건을 받는다. 안 오면 실패다.
func recvDelivery(t *testing.T, ch <-chan Delivery) Delivery {
	t.Helper()
	select {
	case d, ok := <-ch:
		if !ok {
			t.Fatal("채널이 닫혔다")
		}
		return d
	case <-time.After(3 * time.Second):
		t.Fatal("이벤트가 오지 않았다")
	}
	return Delivery{}
}
