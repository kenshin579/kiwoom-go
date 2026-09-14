package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// condPush 는 조건검색 편입·이탈 푸시 한 건이다. 스펙의 ka10173 응답 예제 그대로다 —
// type 은 02 고정이고, 어느 조건검색식인지는 values 의 841 만이 말해 준다.
func condPush(seq, item, insertDelete string) map[string]any {
	return map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "02", "name": "조건검색", "item": item,
			"values": map[string]any{
				"841": seq, "9001": item, "843": insertDelete, "20": "152028", "907": "2",
			},
		}},
	}
}

// 조건검색 푸시가 일련번호로 갈리는지 본다. (타입, 종목)으로는 갈릴 수가 없다 —
// type 은 02 고정이고 종목은 편입이 일어나야 정해진다.
func TestSubscribeCondition_일련번호로_라우팅한다(t *testing.T) {
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

	sub, err := c.SubscribeCondition(ctx, "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}

	ws := <-srvCh
	if err := f.writeJSON(ws, condPush("4", "005930", "I")); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	select {
	case d := <-sub:
		if d.Err != nil {
			t.Fatalf("Err = %v", d.Err)
		}
		if d.Item != "005930" || d.Type != "02" {
			t.Errorf("Type/Item = %s/%s, 기대 02/005930", d.Type, d.Item)
		}
		var vals map[string]string
		if err := json.Unmarshal(d.Values, &vals); err != nil {
			t.Fatalf("values 파싱: %v", err)
		}
		if vals["843"] != "I" || vals["841"] != "4" {
			t.Errorf("values = %v, 기대 841=4 843=I", vals)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("조건검색 푸시가 오지 않았다")
	}
}

// 등록 패킷을 보내지 않아야 한다. 조건검색은 CNSRREQ 요청 자체가 등록이라,
// 여기서 REG 를 보내면 서버에 없는 실시간 항목("조건검색")을 등록하려 드는 셈이 된다.
func TestSubscribeCondition_REG_를_보내지_않는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.SubscribeCondition(ctx, "4", 8); err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}
	// 보냈다면 이 사이에 도착한다. waitPacket 은 실패를 Fatal 로 내므로 직접 훑는다.
	time.Sleep(300 * time.Millisecond)
	for _, p := range f.packets() {
		if p["trnm"] == "REG" || p["trnm"] == "REMOVE" {
			t.Fatalf("%v 를 보냈다 — 조건검색은 요청 자체가 등록이다", p["trnm"])
		}
	}
}

// 남의 조건검색식 푸시를 받으면 안 된다. 열쇠가 하나뿐이라 여기서 새면 조건검색식
// 여럿을 동시에 돌릴 수 없다.
func TestSubscribeCondition_다른_일련번호는_받지_않는다(t *testing.T) {
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

	four, err := c.SubscribeCondition(ctx, "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition(4): %v", err)
	}
	seven, err := c.SubscribeCondition(ctx, "7", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition(7): %v", err)
	}

	ws := <-srvCh
	if err := f.writeJSON(ws, condPush("7", "000660", "D")); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	select {
	case d := <-seven:
		if d.Item != "000660" {
			t.Errorf("Item = %q, 기대 000660", d.Item)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("7번 조건검색식 푸시가 오지 않았다")
	}
	select {
	case d := <-four:
		t.Fatalf("4번 채널에 남의 푸시가 왔다: %+v", d)
	default:
	}
}

// 실시간 구독의 (타입, 종목) 라우팅을 깨지 않는지 본다.
//
// 두 가지를 함께 본다. 실시간 푸시는 실시간 채널로만 가야 하고(841 이 없으니 조건검색
// 쪽은 조용해야 한다), 조건검색 푸시는 조건검색 채널로만 가야 한다.
func TestSubscribeCondition_실시간_라우팅을_깨지_않는다(t *testing.T) {
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

	real0B, err := c.Subscribe(ctx, "0B", []string{"005930"}, 8)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cond, err := c.SubscribeCondition(ctx, "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}

	ws := <-srvCh
	err = f.writeJSON(ws, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "0B", "name": "주식체결", "item": "005930",
			"values": map[string]any{"10": "-82000", "20": "093015"},
		}},
	})
	if err != nil {
		t.Fatalf("실시간 푸시 전송: %v", err)
	}
	if err := f.writeJSON(ws, condPush("4", "005930", "I")); err != nil {
		t.Fatalf("조건검색 푸시 전송: %v", err)
	}

	select {
	case d := <-real0B:
		if d.Type != "0B" {
			t.Errorf("실시간 채널이 받은 Type = %q, 기대 0B", d.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("실시간 푸시가 오지 않았다 — 조건검색 라우팅이 실시간을 깼다")
	}
	select {
	case d := <-cond:
		if d.Type != "02" {
			t.Errorf("조건검색 채널이 받은 Type = %q, 기대 02", d.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("조건검색 푸시가 오지 않았다")
	}
	// 각 채널은 제 것 하나씩만 받았어야 한다.
	select {
	case d := <-real0B:
		t.Errorf("실시간 채널에 조건검색 푸시가 샜다: %+v", d)
	default:
	}
	select {
	case d := <-cond:
		t.Errorf("조건검색 채널에 실시간 푸시가 샜다: %+v", d)
	default:
	}
}

// ctx 취소가 라우팅을 떼고 채널을 닫는지 본다. 닫히지 않으면 소비자는 for range 에서
// 영원히 막힌다.
func TestSubscribeCondition_ctx_취소가_채널을_닫는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	defer func() { _ = c.Close() }()
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sub, err := c.SubscribeCondition(ctx, "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}
	cancel()

	select {
	case _, ok := <-sub:
		if ok {
			for range sub {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}

	// 지도에서도 빠져야 한다. 세는 것까지 내려야 routeReal 이 실시간 푸시마다 841 을
	// 꺼내는 일로 돌아가지 않는다.
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

// 연결이 닫혀도 채널이 닫혀야 한다. 그러지 않으면 이 고루틴이 영영 남는다
// (Subscribe 와 같은 규칙).
func TestSubscribeCondition_Close_가_채널을_닫는다(t *testing.T) {
	f := newFakeServer(t)
	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	sub, err := c.SubscribeCondition(context.Background(), "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}
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

// **이 Task 의 핵심 중 하나다.** 조건검색만 구멍을 숨기면 안 된다.
//
// 조건검색 등록은 재연결이 되살리지 못한다(REG 로 되살릴 수 있는 등록이 아니다).
// 그러니 이 신호가 오지 않으면 사용자는 아무것도 오지 않는 채널을 "조건에 걸린 종목이
// 없다" 로 읽는다 — 영영.
func TestSubscribeCondition_끊겼다_붙으면_구멍이_온다(t *testing.T) {
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

	sub, err := c.SubscribeCondition(ctx, "4", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
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
				return
			}
		case <-timeout:
			t.Fatal("조건검색 구독에 구멍(GapError)이 오지 않았다")
		}
	}
}

// 빈 일련번호는 거절해야 한다. 열쇠가 없으면 어떤 푸시도 짝이 맞지 않아 채널이 영영
// 조용하고, 소비자는 그것을 "조건에 걸린 종목이 없다" 와 구분하지 못한다.
func TestSubscribeCondition_빈_일련번호를_거절한다(t *testing.T) {
	c := New("ws://never", "", &stubToken{token: "TKN"})
	if _, err := c.SubscribeCondition(context.Background(), "", 8); !errors.Is(err, errNoSeq) {
		t.Fatalf("err = %v, 기대 errNoSeq", err)
	}
}

// fieldOf 는 값 하나가 문자열이 아니어도 버티어야 한다. 맵 전체를 map[string]string 으로
// 풀면 그런 푸시 하나가 라우팅을 통째로 멈춘다 — 가장 알아채기 어려운 침묵이다.
func TestFieldOf_문자열이_아닌_값에도_견딘다(t *testing.T) {
	cases := []struct {
		name, values, want string
	}{
		{"문자열", `{"841":"4"}`, "4"},
		{"숫자", `{"841":4}`, "4"},
		{"이웃이 숫자", `{"841":"4","907":2}`, "4"},
		{"없음", `{"9001":"005930"}`, ""},
		{"깨진 JSON", `{`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldOf(json.RawMessage(tc.values), condSeqFID); got != tc.want {
				t.Errorf("fieldOf(%s) = %q, 기대 %q", tc.values, got, tc.want)
			}
		})
	}
}
