package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// **Important 2 의 자리다.** 841 이 빠진 조건검색 푸시가 흔적 없이 사라지면 안 된다.
//
// 라우팅 열쇠는 841 하나뿐이다. 그것이 없으면 이 푸시는 갈 곳이 없고, (타입, 종목) 쪽도
// 비어 있다 — 실시간 지도에 02·S2 타입은 없다. 예전에는 여기서 그냥 버렸다. 로그도
// 카운터도 에러도 없어서, 사용자가 보는 것은 **아무 일도 없는 채널**이었다. 그것을
// "조건에 걸린 종목이 없다" 와 구분할 방법이 없다.
func TestRouteReal_841_이_없는_조건검색_푸시는_에러로_온다(t *testing.T) {
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
	// 841 만 뺀 푸시다. 나머지는 스펙 예제 그대로.
	err = f.writeJSON(ws, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "02", "name": "조건검색", "item": "005930",
			"values": map[string]any{"9001": "005930", "843": "I", "20": "152028", "907": "2"},
		}},
	})
	if err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	d := recvDelivery(t, sub)
	var ue *UnroutedConditionPushError
	if !errors.As(d.Err, &ue) {
		t.Fatalf("기대 *UnroutedConditionPushError, 실제 %+v — 푸시가 조용히 사라졌다", d)
	}
	if ue.Seq != "" {
		t.Errorf("Seq = %q, 기대 빈 문자열(841 이 아예 없었다는 뜻)", ue.Seq)
	}
	if ue.Item != "005930" || ue.Type != "02" {
		t.Errorf("Type/Item = %s/%s, 기대 02/005930", ue.Type, ue.Item)
	}
	// 사람이 읽는 문장에 무슨 일이 벌어졌는지 있어야 한다.
	if !strings.Contains(ue.Error(), "841") {
		t.Errorf("문장에 FID 841 이 없다: %s", ue.Error())
	}
}

// 841 은 있는데 아무 구독의 것도 아닌 경우다. 이쪽도 버리면 안 된다 —
// 해제(CNSRCLR)를 보내지 않고 다시 요청해 일련번호가 갈린 상황이 실제로 여기다.
func TestRouteReal_짝이_없는_일련번호도_에러로_온다(t *testing.T) {
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
	if err := f.writeJSON(ws, condPush("9", "000660", "D")); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	d := recvDelivery(t, sub)
	var ue *UnroutedConditionPushError
	if !errors.As(d.Err, &ue) {
		t.Fatalf("기대 *UnroutedConditionPushError, 실제 %+v", d)
	}
	if ue.Seq != "9" {
		t.Errorf("Seq = %q, 기대 9", ue.Seq)
	}
	if !strings.Contains(ue.Error(), "9") {
		t.Errorf("문장에 일련번호가 없다: %s", ue.Error())
	}
}

// 미국 푸시도 같아야 한다. 스펙 예제상 국내·미국 둘 다 name 이 "조건검색" 으로 온다 —
// 그것이 이 판정의 유일한 열쇠다.
func TestRouteReal_미국_조건검색_푸시도_같은_에러를_준다(t *testing.T) {
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

	sub, err := c.SubscribeCondition(ctx, "2", 8)
	if err != nil {
		t.Fatalf("SubscribeCondition: %v", err)
	}

	ws := <-srvCh
	err = f.writeJSON(ws, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "S2", "name": "조건검색", "item": "COIN", "stexTp": "ND",
			"values": map[string]any{"20": "230156", "843": "I", "907": "2", "9001": "COIN"},
		}},
	})
	if err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	d := recvDelivery(t, sub)
	var ue *UnroutedConditionPushError
	if !errors.As(d.Err, &ue) {
		t.Fatalf("기대 *UnroutedConditionPushError, 실제 %+v", d)
	}
	if ue.Type != "S2" || ue.Item != "COIN" {
		t.Errorf("Type/Item = %s/%s, 기대 S2/COIN", ue.Type, ue.Item)
	}
}

// 짝이 맞는 푸시는 그대로 와야 한다. 못 간 것을 알리자고 간 것까지 흔들면 안 된다.
func TestRouteReal_짝이_맞으면_에러를_흘리지_않는다(t *testing.T) {
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

	d := recvDelivery(t, sub)
	if d.Err != nil {
		t.Fatalf("짝이 맞는 푸시에 에러가 붙었다: %v", d.Err)
	}
	if d.Item != "005930" {
		t.Errorf("Item = %q, 기대 005930", d.Item)
	}
}

// 실시간 푸시는 이 판정에 걸리지 않아야 한다. name 이 "조건검색" 이 아니면 남이다 —
// 구독자 없는 실시간 푸시마다 조건검색 채널에 에러가 쏟아지면 그것이 새 소음이 된다.
func TestRouteReal_구독자_없는_실시간_푸시는_조건검색을_건드리지_않는다(t *testing.T) {
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
	// 아무도 구독하지 않은 0B 푸시다.
	err = f.writeJSON(ws, map[string]any{
		"trnm": "REAL",
		"data": []any{map[string]any{
			"type": "0B", "name": "주식체결", "item": "005930",
			"values": map[string]any{"10": "-82000"},
		}},
	})
	if err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}
	// 그 뒤에 짝이 맞는 조건검색 푸시를 보낸다. 첫 건이 이것보다 먼저 오면 샌 것이다.
	if err := f.writeJSON(ws, condPush("4", "005930", "I")); err != nil {
		t.Fatalf("푸시 전송: %v", err)
	}

	d := recvDelivery(t, sub)
	if d.Err != nil {
		t.Fatalf("조건검색 채널에 먼저 온 것이 에러다: %v — 실시간 푸시가 샜다", d.Err)
	}
}

// 조건검색 채널에 오는 구멍 신호는 *GapError 하나가 아니다.
//
// 세 신호 중 GapError 만 조건검색 테스트가 잡고 있었다. 나머지 둘이 조건검색 자리를
// 건너뛰어도 아무도 모르는 상태라, 여기서 못 박는다. 셋이 다 와야 "조건검색만 구멍을
// 숨기지 않는다" 가 사실이 된다.
func TestSubscribeCondition_느린_소비자도_조건검색_채널로_온다(t *testing.T) {
	c := &Conn{subs: map[subKey][]*sub{}}
	ch := make(chan Delivery, 1)
	s := &sub{key: conditionKey("4"), ch: ch}
	c.subs[s.key] = append(c.subs[s.key], s)

	c.deliver(s, Delivery{Type: "02", Item: "005930"}) // 버퍼를 채운다
	c.deliver(s, Delivery{Type: "02", Item: "000660"}) // 버릴 수밖에 없다
	if s.dropped != 1 {
		t.Fatalf("dropped = %d, 기대 1", s.dropped)
	}

	<-ch                                               // 자리를 낸다
	c.deliver(s, Delivery{Type: "02", Item: "035420"}) // 알림이 먼저 나가야 한다

	d := recvNow(t, ch)
	var slow *SlowConsumerError
	if !errors.As(d.Err, &slow) {
		t.Fatalf("기대 *SlowConsumerError, 실제 %+v", d)
	}
	// 사람이 읽는 문장이 "조건검색/4 구독이 느려…" 가 되어야 한다. 여기에 실시간 코드가
	// 나오면 사용자는 자기가 만들지 않은 무언가를 본 것이 된다.
	if slow.Type != conditionType || slow.Item != "4" {
		t.Errorf("Type/Item = %s/%s, 기대 %s/4", slow.Type, slow.Item, conditionType)
	}
	if slow.Dropped != 1 {
		t.Errorf("Dropped = %d, 기대 1", slow.Dropped)
	}
}

// 아직 못 붙고 있다는 신호도 조건검색 채널로 와야 한다.
//
// 조건검색은 재연결이 등록을 되살리지 못한다 — 그래서 이 신호가 없으면 "장이 조용하다"
// 와 "한 시간째 못 붙고 있다" 에 더해 "등록이 죽었다" 까지 한 덩어리가 된다.
func TestSubscribeCondition_재연결_중_신호도_조건검색_채널로_온다(t *testing.T) {
	c := &Conn{subs: map[subKey][]*sub{}}
	cond := make(chan Delivery, 1)
	real0B := make(chan Delivery, 1)
	cs := &sub{key: conditionKey("4"), ch: cond}
	rs := &sub{key: subKey{"0B", "005930"}, ch: real0B}
	c.subs[cs.key] = append(c.subs[cs.key], cs)
	c.subs[rs.key] = append(c.subs[rs.key], rs)

	c.notifyReconnecting(time.Now(), 3, errors.New("dial 실패"))

	for name, ch := range map[string]chan Delivery{"조건검색": cond, "실시간": real0B} {
		d := recvNow(t, ch)
		var re *ReconnectingError
		if !errors.As(d.Err, &re) {
			t.Errorf("%s 채널: 기대 *ReconnectingError, 실제 %+v", name, d)
		}
	}
}

// **조건검색의 구멍은 실시간의 구멍과 뜻이 다르다.**
//
// 실시간은 다시 붙으면서 REG 가 복구되지만, 조건검색은 요청 자체가 등록이라 되살릴 REG 가
// 없다 — 받은 쪽은 요청을 다시 보내야 한다. 같은 타입에 같은 문장으로 오면 ev.Err != nil
// 하나로 처리하는 핸들러가 둘을 구분하지 못한다. 값으로 갈린다.
func TestNotifyGap_조건검색과_실시간의_구멍이_값으로_갈린다(t *testing.T) {
	c := &Conn{subs: map[subKey][]*sub{}}
	cond := make(chan Delivery, 1)
	real0B := make(chan Delivery, 1)
	cs := &sub{key: conditionKey("4"), ch: cond}
	rs := &sub{key: subKey{"0B", "005930"}, ch: real0B}
	c.subs[cs.key] = append(c.subs[cs.key], cs)
	c.subs[rs.key] = append(c.subs[rs.key], rs)

	c.notifyGap(time.Now().Add(-time.Second), time.Now())

	dc := recvNow(t, cond)
	var gc *GapError
	if !errors.As(dc.Err, &gc) {
		t.Fatalf("조건검색 채널: 기대 *GapError, 실제 %+v", dc)
	}
	if gc.Resubscribed {
		t.Error("조건검색의 구멍이 Resubscribed=true 로 왔다 — 되살릴 REG 가 없는 등록이다")
	}
	if !strings.Contains(gc.Error(), "다시 보내라") {
		t.Errorf("문장이 무엇을 해야 하는지 말하지 않는다: %s", gc.Error())
	}

	dr := recvNow(t, real0B)
	var gr *GapError
	if !errors.As(dr.Err, &gr) {
		t.Fatalf("실시간 채널: 기대 *GapError, 실제 %+v", dr)
	}
	if !gr.Resubscribed {
		t.Error("실시간의 구멍이 Resubscribed=false 로 왔다 — REG 는 다시 보냈다")
	}
	if strings.Contains(gr.Error(), "다시 보내라") {
		t.Errorf("실시간 구멍이 조건검색의 문장을 쓴다: %s", gr.Error())
	}
}

// **Important 4 의 자리다.** 푸시 봉투의 stexTp 는 values **밖**에 붙는다.
//
// 읽지 않으면 미국 종목의 거래소 구분을 어떤 경로로도 얻을 수 없다 — 조회 응답에서는
// 철자가 갈리는 것까지 받아 내는 값이다. 그 철자 갈림은 푸시에도 그대로 적용한다.
func TestRouteReal_푸시_봉투의_거래소구분을_두_철자로_싣는다(t *testing.T) {
	for _, key := range []string{"stexTp", "stex_tp"} {
		t.Run(key, func(t *testing.T) {
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

			sub, err := c.SubscribeCondition(ctx, "2", 8)
			if err != nil {
				t.Fatalf("SubscribeCondition: %v", err)
			}

			ws := <-srvCh
			elem := map[string]any{
				"type": "S2", "name": "조건검색", "item": "COIN",
				"values": map[string]any{
					"20": "230156", "841": "2", "843": "I", "907": "2", "9001": "COIN",
				},
			}
			elem[key] = "ND"
			if err := f.writeJSON(ws, map[string]any{"trnm": "REAL", "data": []any{elem}}); err != nil {
				t.Fatalf("푸시 전송: %v", err)
			}

			d := recvDelivery(t, sub)
			if d.Err != nil {
				t.Fatalf("Err = %v", d.Err)
			}
			if d.StexTp != "ND" {
				t.Errorf("StexTp = %q, 기대 ND — 봉투의 거래소구분이 사라졌다", d.StexTp)
			}
		})
	}
}
