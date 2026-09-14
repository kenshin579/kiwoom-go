package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Delivery 는 라우팅된 실시간 한 건이다.
//
// Values 를 맵으로 풀지 않고 원문으로 넘기는 이유: 호출자가 같은 원문을 두 번 읽어
// (1) 타입 구조체와 (2) Raw 맵을 함께 채운다. 여기서 맵으로 풀면 타입 쪽이 다시
// 직렬화를 거쳐야 한다.
type Delivery struct {
	Type   string          // 실시간 항목 (0B 등)
	Name   string          // 실시간 항목명
	Item   string          // 종목코드
	Values json.RawMessage // FID → 값
	Time   time.Time       // 수신 시각
	// Err 은 *GapError · *ReconnectingError · *SlowConsumerError 다.
	// 이때 나머지 필드는 비어 있다.
	Err error
}

// realPayload 는 REAL 메시지의 data 원소다.
//
// values 는 **맵**이다(FID 숫자가 키). 스펙은 LIST 라고 적었지만 문서 오류다 —
// tools/spec/SOURCE.md 의 "WebSocket 규약" 절 참고.
type realPayload struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Item   string          `json:"item"`
	Values json.RawMessage `json:"values"`
}

type subKey struct{ typ, item string }

// sub 은 (타입, 종목) 하나에 대한 구독 자리다.
//
// pendingGap·dropped·closed 는 c.subMu 가 지킨다. 수신 고루틴(routeReal)과 해지 고루틴이
// 같은 자리를 함께 만지므로 채널 송신까지 그 락 안에서 한다 — 송신은 논블로킹이라
// 락을 들고 있어도 막히지 않는다.
type sub struct {
	key subKey
	ch  chan Delivery

	// pendingGap 은 아직 흘리지 못한 구멍이다. 자리가 나는 첫 순간에 보낸다.
	//
	// dropped 와 나란히 있지만 **뜻이 다르다.** dropped 는 "느려서 버렸다" 고, 이것은
	// "그 사이 연결이 끊겼다" 다. 자리가 없다고 이것을 dropped 로 돌리면 사용자는
	// 끊겼다는 사실을 영영 모르고, 대신 사실이 아닌 느림 경고를 받는다.
	pendingGap *GapError
	dropped    int
	closed     bool
}

// latchGap 은 구멍을 래치에 얹는다. 이미 있으면 구간을 합친다 — 겹친 구멍 둘을
// 따로 알릴 방법이 없다면, 넓은 쪽으로 합치는 편이 좁게 잘라 말하는 것보다 정직하다.
//
// 자리마다 제 복사본을 든다. 하나를 여럿이 나눠 들면 합치는 순간 남의 구간까지 넓힌다.
// subMu 를 들고 부른다.
func (s *sub) latchGap(since, until time.Time) {
	if s.pendingGap == nil {
		s.pendingGap = &GapError{Since: since, Until: until}
		return
	}
	if since.Before(s.pendingGap.Since) {
		s.pendingGap.Since = since
	}
	if until.After(s.pendingGap.Until) {
		s.pendingGap.Until = until
	}
}

// flushGap 은 래치의 구멍을 흘린다. 흘렸거나 흘릴 것이 없으면 true.
// subMu 를 들고 부른다.
func (s *sub) flushGap() bool {
	if s.pendingGap == nil {
		return true
	}
	select {
	case s.ch <- Delivery{Err: s.pendingGap, Time: s.pendingGap.Until}:
		s.pendingGap = nil
		return true
	default:
		return false
	}
}

// errNoItems 는 종목 없이 구독하려 했다는 뜻이다.
//
// 구독 자리는 (타입, 종목)으로 잡힌다 — 종목이 없으면 자리가 하나도 생기지 않아 어떤
// 실시간도 이 채널로 오지 않고, 해지될 때 닫을 자리도 없어 채널이 영영 열린 채 남는다.
// 소비자는 for range 에서 영원히 막힌다. 조용히 그렇게 두느니 여기서 거절한다.
var errNoItems = errors.New("kiwoom: 구독할 종목이 없다 — items 가 비어 있다")

// Subscribe 는 실시간 등록(REG)을 보내고 채널을 준다.
//
// ctx 가 취소되면 해지(REMOVE)를 보내고 채널을 닫는다. 연결이 Close 되어도 닫는다 —
// 그러지 않으면 이 고루틴이 영영 남는다. buf 는 채널 버퍼 크기다 — 가득 차면
// 이벤트를 버리고 *SlowConsumerError 를 흘린다. 소켓 전체를 막지 않는다.
//
// items 가 비어 있으면 에러다(errNoItems).
func (c *Conn) Subscribe(ctx context.Context, typ string, items []string, buf int) (<-chan Delivery, error) {
	if len(items) == 0 {
		return nil, errNoItems
	}
	if buf < 1 {
		buf = 1
	}
	// **연결은 여기서 붙는다.** 사용자는 Connect 를 부르지 않는다 — 부를 방법도 없다
	// (Conn 은 internal/ 안에 있다). 구독 자리를 잡기 **전에** 붙는 이유는, 실패했을 때
	// 지도에 남는 찌꺼기가 없어야 하기 때문이다.
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}
	ch := make(chan Delivery, buf)

	c.subMu.Lock()
	if c.subs == nil {
		c.subs = map[subKey][]*sub{}
	}
	made := make([]*sub, 0, len(items))
	for _, it := range items {
		s := &sub{key: subKey{typ, it}, ch: ch}
		c.subs[s.key] = append(c.subs[s.key], s)
		made = append(made, s)
	}
	// regs 는 재연결이 다시 보낼 등록이다. 해지되면 여기서도 빼야 한다 —
	// 남겨 두면 재연결이 구독자 없는 종목을 계속 다시 등록한다.
	reg := &registration{typ: typ, items: items}
	c.regs = append(c.regs, reg)
	c.subMu.Unlock()

	if err := c.sendReg(ctx, "REG", typ, items); err != nil {
		c.dropReg(reg)
		c.closeSubs(made, ch)
		return nil, err
	}

	life := c.lifeCtx()
	go func() {
		select {
		case <-ctx.Done():
		case <-life.Done():
		}
		// regs 에서 **먼저** 뺀다. REMOVE 를 보내는 사이에 재연결이 끼어들면
		// 방금 해지한 것을 다시 등록해 버린다.
		c.dropReg(reg)
		// 해지는 best-effort 다. 연결이 이미 죽었으면 보낼 곳이 없다.
		// 그래도 채널은 반드시 닫는다.
		rmCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = c.sendReg(rmCtx, "REMOVE", typ, items)
		cancel()
		c.closeSubs(made, ch)
	}()

	return ch, nil
}

// registration 은 재연결 때 다시 보낼 등록이다.
//
// 값이 아니라 포인터로 들고 다닌다 — 해지할 때 "내가 넣은 그것" 을 지워야 하는데,
// 같은 (typ, items) 로 두 번 구독할 수 있어 값 비교로는 남의 것을 지운다.
type registration struct {
	typ   string
	items []string
}

// dropReg 는 해지된 등록을 재연결 목록에서 뺀다.
func (c *Conn) dropReg(r *registration) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for i, x := range c.regs {
		if x == r {
			c.regs = append(c.regs[:i:i], c.regs[i+1:]...)
			return
		}
	}
}

func (c *Conn) sendReg(ctx context.Context, trnm, typ string, items []string) error {
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return errNotConnected
	}
	return writeJSON(ctx, ws, map[string]any{
		"trnm":    trnm,
		"grp_no":  "1",
		"refresh": "1",
		// item·type 은 **배열**이다. 스펙의 depth-1 String 선언과 다르다(SOURCE.md 참고).
		"data": []any{map[string]any{"item": items, "type": []string{typ}}},
	})
}

// closeSubs 는 구독 자리를 지도에서 빼고 채널을 닫는다.
//
// 빼는 것과 닫는 것을 **한 락 안에서** 한다. 따로 하면 그 틈에 routeReal 이 집어간
// 자리로 닫힌 채널에 보내다 죽는다. deliver 도 같은 락에서 closed 를 본다.
func (c *Conn) closeSubs(ss []*sub, ch chan Delivery) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	already := true
	for _, s := range ss {
		if !s.closed {
			already = false
		}
		s.closed = true
		list := c.subs[s.key]
		for i, x := range list {
			if x == s {
				c.subs[s.key] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
		if len(c.subs[s.key]) == 0 {
			delete(c.subs, s.key)
		}
	}
	if !already {
		close(ch)
	}
}

// routeReal 은 REAL 메시지를 구독자에게 나눠 준다.
func (c *Conn) routeReal(env envelope) {
	var payloads []realPayload
	if err := json.Unmarshal(env.Data, &payloads); err != nil {
		return
	}
	now := time.Now()
	for _, p := range payloads {
		c.subMu.Lock()
		list := append([]*sub(nil), c.subs[subKey{p.Type, p.Item}]...)
		c.subMu.Unlock()
		for _, s := range list {
			c.deliver(s, Delivery{
				Type: p.Type, Name: p.Name, Item: p.Item,
				Values: p.Values, Time: now,
			})
		}
	}
}

// deliver 는 막히지 않고 보낸다. 가득 차면 버리고, 버린 사실을 알린다.
//
// 이것이 느린 소비자가 소켓 전체를 막지 못하게 하는 자리다 — 한 구독의 느림이
// 다른 구독을 굶기면 안 된다. 송신은 전부 select+default 라 수신 고루틴이 여기서
// 멈추는 일이 없다.
//
// 알림은 **다음 번에** 자리가 나면 흘린다. 가득 찼을 때 알림을 밀어 넣으려 해 봐야
// 그것도 가득 차 있어 같이 버려지고, 그러면 소비자는 자기가 놓친 것을 영영 모른다.
// 그래서 버린 건수를 들고 있다가 자리가 나는 첫 순간에 한 번에 알린다. 구멍도 같은
// 방식으로 래치에 들고 있다가 흘린다.
func (c *Conn) deliver(s *sub, d Delivery) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if s.closed {
		return
	}

	// 밀린 알림이 데이터보다 **먼저** 가야 한다. 구멍 뒤에 온 값을 구멍 전의 것으로
	// 읽으면 사용자가 메꿀 구간을 잘못 잡는다.
	if !c.flushNotices(s) {
		// 아직도 자리가 없다 — 이번 것도 버리고 다음을 기약한다.
		s.dropped++
		return
	}

	select {
	case s.ch <- d:
	default:
		s.dropped++
	}
}

// flushNotices 는 밀린 알림을 순서대로 흘린다. 전부 흘렸으면 true.
//
// 구멍이 먼저다. 끊긴 것이 느린 것보다 무겁고, 자리가 한 칸뿐일 때 하나만 통과한다면
// 그것은 구멍이어야 한다 — 느림은 다음에 또 알릴 수 있지만 구멍은 지금 이 한 건이다.
// subMu 를 들고 부른다.
func (c *Conn) flushNotices(s *sub) bool {
	if !s.flushGap() {
		return false
	}
	if s.dropped > 0 {
		select {
		case s.ch <- Delivery{
			Time: time.Now(),
			Err:  &SlowConsumerError{Type: s.key.typ, Item: s.key.item, Dropped: s.dropped},
		}:
			s.dropped = 0
		default:
			return false
		}
	}
	return true
}
