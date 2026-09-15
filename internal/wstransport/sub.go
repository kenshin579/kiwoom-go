package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Delivery 는 라우팅된 실시간 한 건이다.
//
// Values 를 맵으로 풀지 않고 원문으로 넘기는 이유: 호출자가 같은 원문을 두 번 읽어
// (1) 타입 구조체와 (2) Raw 맵을 함께 채운다. 여기서 맵으로 풀면 타입 쪽이 다시
// 직렬화를 거쳐야 한다.
type Delivery struct {
	Type   string          // 실시간 항목 (0B·02·S2 등)
	Name   string          // 실시간 항목명
	Item   string          // 종목코드
	StexTp string          // 거래소구분. 미국 조건검색 푸시에만 온다 — realPayload 의 주석 참고
	Values json.RawMessage // FID → 값
	Time   time.Time       // 수신 시각
	// Err 은 *GapError · *ReconnectingError · *SlowConsumerError ·
	// *UnroutedConditionPushError 다. 이때 나머지 필드는 비어 있다.
	Err error
}

// realPayload 는 REAL 메시지의 data 원소다.
//
// values 는 **맵**이다(FID 숫자가 키). 스펙은 LIST 라고 적었지만 문서 오류다 —
// tools/spec/SOURCE.md 의 "WebSocket 규약" 절 참고.
//
// StexTp 는 values **바깥**에 붙는 봉투 필드다. usa20290 의 푸시 예제에만 나오고
// (`"stexTp":"ND"`) 실시간 23종에는 없다. 읽지 않으면 미국 종목의 거래소 구분을 어떤
// 경로로도 얻을 수 없어 — 조회 응답에서는 커스텀 UnmarshalJSON 까지 써서 얻는 값이다 —
// 여기서 봉투째 싣고 stream.Event 의 같은 이름 자리로 넘긴다. FID 가 아니므로 Raw 에
// 섞지 않는다(Raw 의 열쇠는 FID 숫자라는 약속이 깨진다).
type realPayload struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Item string `json:"item"`
	// 거래소구분을 두 철자로 받는다. 푸시 예제는 stexTp 지만 같은 API 의 조회 응답은
	// stex_tp 로 온다 — 서버가 같은 값을 두 철자로 쓰는 자리라, 하나만 받으면 반대쪽이
	// 올 때 조용히 빈다. 조회 응답 쪽도 같은 이유로 둘 다 받는다
	// (overseas/condition 의 RequestOverseasRealtimeConditionSearchDataItem.UnmarshalJSON).
	StexTpCamel string          `json:"stexTp"`
	StexTpSnake string          `json:"stex_tp"`
	Values      json.RawMessage `json:"values"`
}

// stexTp 는 두 철자 중 온 것을 돌려준다.
func (p realPayload) stexTp() string {
	if p.StexTpCamel != "" {
		return p.StexTpCamel
	}
	return p.StexTpSnake
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
		// Resubscribed 로 실시간과 조건검색을 가른다. 실시간은 재연결이 REG 를 다시 보내
		// 등록이 살아 있지만, 조건검색은 요청 자체가 등록이라 되살릴 REG 가 없다 —
		// 같은 타입에 같은 문장으로 오면 ev.Err != nil 하나로 처리하는 핸들러가 둘을
		// 구분하지 못한다. GapError 의 주석 참고.
		s.pendingGap = &GapError{
			Since: since, Until: until,
			Resubscribed: s.key.typ != conditionType,
		}
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
			// 조건검색 자리는 센다. 이 수가 0 이면 routeReal 이 841 을 꺼내지 않는다 —
			// 내리는 것을 빠뜨리면 조건검색을 한 번 쓴 연결이 그 뒤로 영원히 실시간
			// 푸시마다 맵 하나를 더 만든다.
			if s.key.typ == conditionType {
				c.condSubs--
			}
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
		cond := c.condSubs > 0
		c.subMu.Unlock()

		// 조건검색 푸시는 같은 REAL 메시지로 오지만 (타입, 종목)으로 갈리지 않는다.
		// 열쇠는 values 의 841(일련번호)이다 — conditionKey 의 주석 참고.
		//
		// 조건검색 구독이 하나도 없으면 풀지 않는다. 실시간 푸시는 초당 여러 건이 오는데
		// 아무도 듣지 않는 열쇠를 꺼내자고 그때마다 맵을 하나씩 만들 이유가 없다.
		seq := ""
		if cond {
			if seq = fieldOf(p.Values, condSeqFID); seq != "" {
				c.subMu.Lock()
				list = append(list, c.subs[conditionKey(seq)]...)
				c.subMu.Unlock()
			}
		}

		// 조건검색 푸시가 어디에도 닿지 못했으면 **말한다.**
		//
		// 841 이 빠졌거나 어느 구독과도 짝이 맞지 않으면 이 푸시는 갈 곳이 없다 —
		// 실시간 지도에도 02·S2 타입은 없으니 (타입, 종목) 쪽도 비어 있다. 예전에는
		// 여기서 그냥 버렸다. 로그도 카운터도 에러도 없어서, 사용자는 그것을
		// "조건에 걸린 종목이 없다" 와 구분할 수 없었다 — 이 라이브러리에서 구멍이
		// 소리 없이 닫히는 유일한 자리였다.
		//
		// 살아 있는 조건검색 구독 전체에 흘린다. 어느 구독의 것이었는지 모르니
		// 고를 수가 없다 — 한 곳에만 흘리려면 그 자체가 추측이 된다.
		//
		// 조건검색 구독이 하나도 없으면(cond == false) 알릴 곳 자체가 없다. 그 상태로도
		// 푸시는 올 수 있다 — ctx 취소는 해제가 아니라서, 채널을 닫고 CNSRCLR 를 보내지
		// 않으면 서버는 계속 민다. 말할 상대가 없는 것이지 감추는 것이 아니다.
		if cond && p.Name == conditionType && len(list) == 0 {
			c.notifyUnroutedCondition(p, seq, now)
			continue
		}

		for _, s := range list {
			c.deliver(s, Delivery{
				Type: p.Type, Name: p.Name, Item: p.Item,
				StexTp: p.stexTp(), Values: p.Values, Time: now,
			})
		}
	}
}

// notifyUnroutedCondition 은 갈 곳을 찾지 못한 조건검색 푸시를 살아 있는 조건검색
// 구독 전체에 에러로 알린다.
//
// 자리를 모아 두고 락 밖에서 deliver 를 탄다 — deliver 가 제 손으로 subMu 를 잡고,
// 닫힘·버림·구멍 래치를 실시간과 **같은 코드**로 처리한다. 채널이 가득 차면 이것도
// 버려지지만 그때는 dropped 가 올라 *SlowConsumerError 로 드러난다. 조용히 사라지는
// 경로가 남지 않는 것이 요점이다.
//
// 한 채널에 여러 자리가 걸려 있으면 한 번만 보낸다(notifyGap 과 같은 규칙).
func (c *Conn) notifyUnroutedCondition(p realPayload, seq string, now time.Time) {
	c.subMu.Lock()
	var targets []*sub
	seen := map[chan Delivery]bool{}
	for key, list := range c.subs {
		if key.typ != conditionType {
			continue
		}
		for _, s := range list {
			if s.closed || seen[s.ch] {
				continue
			}
			seen[s.ch] = true
			targets = append(targets, s)
		}
	}
	c.subMu.Unlock()

	if len(targets) == 0 {
		return
	}
	err := &UnroutedConditionPushError{
		Type: p.Type, Name: p.Name, Item: p.Item, Seq: seq,
	}
	for _, s := range targets {
		c.deliver(s, Delivery{Err: err, Time: now})
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

// conditionType 은 조건검색 구독 자리의 타입 열쇠다.
//
// 실시간 항목 코드가 **아니다.** 실시간 코드는 두 글자(0B·S2…)라 이 낱말과 부딪힐 수
// 없고, 그래서 (타입, 종목) 지도에 함께 살아도 서로를 가리지 않는다.
//
// 사람이 읽는 문장에 그대로 나온다("kiwoom: 조건검색/4 구독이 느려…"). 그 자리에
// "__cond" 같은 내부 표식이 나오면 사용자는 자기가 만들지 않은 무언가를 본 것이 된다.
const conditionType = "조건검색"

// condSeqFID 는 조건검색 푸시에서 조건검색식 일련번호를 싣는 FID 다.
//
// tools/gen/fids.go 의 표에서는 SequenceNumber(일련번호)다. 스펙의 ka10173 실시간 절과
// usa20290 응답 예제가 둘 다 이 자리에 요청의 seq 를 그대로 돌려준다.
const condSeqFID = "841"

// conditionKey 는 조건검색 푸시의 라우팅 열쇠다.
//
// 실시간은 (타입, 종목)으로 갈리지만 조건검색은 그럴 수가 없다 — 푸시의 type 은
// 02(국내)·S2(미국) 고정이고, 종목은 편입·이탈이 일어나야 정해져 구독 시점에는 모른다.
// 갈리는 것은 **조건검색식 일련번호** 하나뿐이다.
func conditionKey(seq string) subKey { return subKey{typ: conditionType, item: seq} }

// errNoSeq 는 일련번호 없이 조건검색 푸시를 받으려 했다는 뜻이다.
//
// errNoItems 와 같은 이유로 거절한다. 빈 열쇠로는 어떤 푸시도 짝이 맞지 않아 채널이
// 영영 조용하고, 소비자는 "조건에 걸린 종목이 없다" 와 구분하지 못한다.
var errNoSeq = errors.New("kiwoom: 조건검색식 일련번호가 비어 있다 — 라우팅 열쇠가 없어 푸시가 한 건도 오지 않는다")

// SubscribeCondition 은 조건검색 편입·이탈 푸시를 받는다.
//
// **등록 패킷(REG)을 보내지 않는다.** 조건검색은 CNSRREQ(ka10173)·GCNSRREQ(usa20290)
// 요청 자체가 등록이다. 여기서 하는 일은 라우팅을 붙이는 것뿐이다. 해지도 REMOVE 가
// 아니라 CNSRCLR(ka10174)·GCNSRCLR(usa20291) 요청으로 따로 보낸다.
//
// 그래서 **재연결이 이 등록을 복구하지 못한다.** c.regs 에 넣지 않는 이유가 그것이다 —
// 넣어 봐야 REG 로는 되살릴 수 없는 등록이라, 다시 붙은 뒤 조용히 REG 를 흘리면 사용자는
// 복구된 줄 알고 오지 않는 푸시를 기다린다. 대신 다른 구독과 똑같이 *GapError 가 온다.
// 그 신호를 받으면 요청을 다시 보내는 것은 호출자의 몫이다.
//
// ctx 가 취소되거나 연결이 닫히면 라우팅을 떼고 채널을 닫는다. 채널이 가득 찰 때의
// 처리(*SlowConsumerError)와 구멍 래치는 실시간 구독과 **같은 코드**를 탄다 —
// 조건검색만 구멍을 숨기지 않는다.
func (c *Conn) SubscribeCondition(ctx context.Context, seq string, buf int) (<-chan Delivery, error) {
	if seq == "" {
		return nil, errNoSeq
	}
	if buf < 1 {
		buf = 1
	}
	// 여기서는 ensureConnected 를 부르지 않는다. 호출자가 방금 같은 소켓으로 요청을
	// 보냈으니 연결은 이미 서 있고, 그 사이에 끊겼다면 새로 다이얼하는 것보다 자리를
	// 잡아 두는 편이 낫다 — 다시 붙는 즉시 *GapError 가 이 채널로 온다.
	ch := make(chan Delivery, buf)
	s := &sub{key: conditionKey(seq), ch: ch}

	c.subMu.Lock()
	if c.subs == nil {
		c.subs = map[subKey][]*sub{}
	}
	c.subs[s.key] = append(c.subs[s.key], s)
	c.condSubs++
	c.subMu.Unlock()

	life := c.lifeCtx()
	go func() {
		select {
		case <-ctx.Done():
		case <-life.Done():
		}
		c.closeSubs([]*sub{s}, ch)
	}()
	return ch, nil
}

// fieldOf 는 values 에서 FID 하나를 문자열로 꺼낸다. 없으면 빈 문자열이다.
//
// map[string]string 으로 한 번에 풀지 않는다. 그러면 값 하나라도 숫자로 오는 순간
// 맵 전체가 파싱에 실패해 라우팅이 통째로 멈춘다 — 푸시는 계속 오는데 아무에게도
// 닿지 않는, 가장 알아채기 어려운 종류의 침묵이 된다.
func fieldOf(values json.RawMessage, fid string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(values, &m); err != nil {
		return ""
	}
	raw, ok := m[fid]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		// 문자열이 아니면 원문 그대로 쓴다. 841 이 4 로 와도 "4" 로 라우팅된다.
		return strings.TrimSpace(string(raw))
	}
	return s
}
