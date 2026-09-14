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
type GapError struct {
	Since time.Time // 끊긴 시각
	Until time.Time // 다시 붙은 시각
}

func (e *GapError) Error() string {
	return fmt.Sprintf("kiwoom: %s ~ %s 동안 실시간이 끊겼다 — 그 구간 이벤트는 받지 못했다",
		e.Since.Format(time.RFC3339), e.Until.Format(time.RFC3339))
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
	Type    string // 실시간 항목 (0B 등)
	Item    string // 종목코드
	Dropped int    // 버린 건수
}

func (e *SlowConsumerError) Error() string {
	return fmt.Sprintf("kiwoom: %s/%s 구독이 느려 %d건을 버렸다 — 채널을 더 빨리 비우거나 버퍼를 늘려라",
		e.Type, e.Item, e.Dropped)
}
