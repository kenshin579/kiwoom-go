// Package wstransport 는 키움 WebSocket 연결을 다룬다.
//
// REST 의 internal/transport 와 형제이고 토큰 발급기(internal/auth)를 공유한다.
// 연결은 경로마다 하나다 — 국내(/api/dostk/websocket)와 미국(/api/us/websocket).
//
// 이 패키지가 지키는 원칙은 하나다: **붙이되 구멍을 숨기지 않는다.** 끊기면 다시 붙고
// 등록을 다시 보내지만, 그 사이 이벤트는 영영 못 받는다. 그것을 *GapError 로 알린다.
// 아직 못 붙고 있는 동안은 *ReconnectingError 로 알린다 — 침묵이 "장이 조용하다" 와
// 구분되지 않으면 안 되기 때문이다.
package wstransport

import (
	"fmt"
	"time"
)

// LoginError 는 로그인 패킷에 서버가 return_code != 0 으로 답한 것이다.
type LoginError struct {
	ReturnCode int
	ReturnMsg  string
}

// Error 의 모양은 APIError 와 맞춘다 — 같은 종류의 실패(return_code)를 두 가지 문장으로
// 적으면 로그에서 같은 것을 찾는 방법이 둘이 된다.
func (e *LoginError) Error() string {
	return fmt.Sprintf("kiwoom: websocket 로그인 실패 (return_code=%d): %s", e.ReturnCode, e.ReturnMsg)
}

// GapError 는 끊겼다가 다시 붙은 구간이다. 그 사이 이벤트는 받지 못했다.
//
// 라이브러리가 대신 메꿔 주지 않는다 — 어떤 REST 호출로 메꿔야 하는지는 종목·타입마다
// 다르고, 우리가 고를 일이 아니다. 사용자가 이 신호를 보고 직접 정한다.
//
// 채널이 가득 차 바로 흘리지 못하면 구독 자리에 래치로 남았다가 자리가 나는 첫 순간에
// 온다. 늦게 올지언정 사라지지는 않는다 — 구멍을 "느림" 으로 둔갑시키면 사용자는
// 끊겼다는 사실 자체를 영영 모른다.
// Resubscribed 는 **뜻이 둘로 갈리는 자리**다. 실시간 구독에서는 다시 붙으면서 등록(REG)이
// 복구되었다는 뜻이라 할 일이 "구간을 메꿀지 정하는 것" 뿐이지만, 조건검색에서는 등록이
// 죽은 채다 — 요청(CNSRREQ·GCNSRREQ) 자체가 등록이라 되살릴 REG 가 없다. 그때는 요청을
// 다시 보내지 않는 한 그 채널로 아무것도 오지 않는다.
//
// 둘을 같은 타입에 같은 문장으로 내면 `ev.Err != nil` 하나로 처리하는 핸들러가 구분하지
// 못한다 — 값에 넣어 갈랐다.
type GapError struct {
	Since time.Time // 끊긴 시각
	Until time.Time // 다시 붙은 시각
	// Resubscribed 는 등록이 자동으로 복구되었는지다.
	//
	// 실시간 구독은 true(REG 를 다시 보냈다), 조건검색은 false(되살릴 REG 가 없다).
	// false 면 요청을 다시 보내야 그 채널이 살아난다.
	Resubscribed bool
}

func (e *GapError) Error() string {
	if !e.Resubscribed {
		return fmt.Sprintf("kiwoom: %s ~ %s 동안 실시간이 끊겼다 — 그 구간 이벤트는 받지 못했고, "+
			"등록이 복구되지 않아 지금은 아무것도 오지 않는다(요청을 다시 보내라)",
			e.Since.Format(time.RFC3339), e.Until.Format(time.RFC3339))
	}
	return fmt.Sprintf("kiwoom: %s ~ %s 동안 실시간이 끊겼다 — 그 구간 이벤트는 받지 못했다",
		e.Since.Format(time.RFC3339), e.Until.Format(time.RFC3339))
}

// UnroutedConditionPushError 는 조건검색 푸시가 왔는데 **갈 곳을 찾지 못했다**는 뜻이다.
//
// 조건검색 푸시는 (타입, 종목)으로 갈리지 않는다 — type 은 02·S2 고정이라 실시간 지도에
// 자리가 없고, 갈리는 것은 values 의 841(일련번호) 하나뿐이다. 그 FID 가 빠져 있거나
// 어느 구독의 일련번호와도 맞지 않으면 이 푸시는 어디에도 닿지 못한다.
//
// 그냥 버리면 사용자는 그것을 **"조건에 걸린 종목이 없다" 와 구분할 수 없다.** 채널은
// 조용하고, 로그도 카운터도 없다. 그래서 살아 있는 조건검색 구독 전체에 이 에러를 흘린다 —
// 어느 구독의 것이었는지는 모르므로 한 곳을 고르지 않는다.
//
// 받았다고 구독이 끝난 것은 아니다. 짝이 맞는 푸시는 그대로 온다.
type UnroutedConditionPushError struct {
	Type string // 푸시의 실시간 항목 (02·S2)
	Name string // 푸시의 실시간 항목명 (조건검색)
	Item string // 푸시의 종목코드
	Seq  string // values 의 841. **비어 있으면 그 FID 가 아예 없었다는 뜻이다**
}

func (e *UnroutedConditionPushError) Error() string {
	if e.Seq == "" {
		return fmt.Sprintf("kiwoom: 조건검색 푸시(type=%s item=%s)에 일련번호(FID 841)가 없어 "+
			"어느 조건검색식의 것인지 가릴 수 없다 — 이 건은 어느 구독에도 닿지 못했다", e.Type, e.Item)
	}
	return fmt.Sprintf("kiwoom: 일련번호 %s 의 조건검색 푸시(type=%s item=%s)를 받을 구독이 없다 — "+
		"이 건은 어느 구독에도 닿지 못했다", e.Seq, e.Type, e.Item)
}

// ReconnectingError 는 **아직 다시 붙지 못했다**는 뜻이다.
//
// GapError 는 붙은 뒤에야 온다. 장애가 이어지는 동안 아무 말도 하지 않으면 구독자는
// "장이 조용하다" 와 "한 시간째 못 붙고 있다" 를 구분할 수 없다. 그래서 몇 번 연달아
// 실패한 뒤부터는 시도마다 이것을 흘린다 — 붙을 때까지 되풀이해서 온다.
//
// 받았다고 구독이 끝난 것은 아니다. 다시 붙으면 등록이 복구되고 GapError 가 뒤따른다.
type ReconnectingError struct {
	Since    time.Time // 끊긴 시각
	Attempts int       // 지금까지 시도한 횟수
	Last     error     // 마지막 시도가 실패한 이유
}

func (e *ReconnectingError) Error() string {
	return fmt.Sprintf("kiwoom: %s 부터 실시간이 끊겨 있다 — %d번 다시 붙으려 했으나 실패했다: %v",
		e.Since.Format(time.RFC3339), e.Attempts, e.Last)
}

// Unwrap 은 마지막 실패 원인을 내준다. errors.Is/As 로 원인을 가릴 수 있다.
func (e *ReconnectingError) Unwrap() error { return e.Last }

// SlowConsumerError 는 구독 채널이 가득 차 이벤트를 버렸다는 뜻이다.
//
// 소켓 전체를 막지 않는다 — 한 구독의 느림이 다른 구독을 굶기면 안 된다.
//
// **끊김은 여기로 세지 않는다.** 구멍을 이 에러로 돌리면 "느려서 버렸다" 는 거짓말이
// 되고, 정작 끊겼다는 사실은 사라진다(GapError 의 래치 참고).
type SlowConsumerError struct {
	// Type 은 실시간 항목(0B 등)이다. 조건검색 구독에서는 실시간 코드가 아니라
	// "조건검색" 이 온다 — 그 구독의 자리는 (타입, 종목)이 아니라 (조건검색, 일련번호)로
	// 잡히기 때문이다(sub.go 의 conditionType).
	Type string
	// Item 은 종목코드다. 조건검색 구독에서는 조건검색식 일련번호가 온다(841).
	Item    string
	Dropped int // 버린 건수
}

func (e *SlowConsumerError) Error() string {
	return fmt.Sprintf("kiwoom: %s/%s 구독이 느려 %d건을 버렸다 — 채널을 더 빨리 비우거나 버퍼를 늘려라",
		e.Type, e.Item, e.Dropped)
}
