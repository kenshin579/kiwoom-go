package kiwoom

import (
	"time"

	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

// Event 는 실시간 한 건이다.
//
// Err 이 nil 이 아니면 Value 는 영값이다 — 구멍(*GapError)이나 느린 소비자
// (*SlowConsumerError)를 알리는 봉투이기 때문이다.
type Event[T any] struct {
	Symbol string // 종목코드
	Name   string // 실시간 항목명
	Time   time.Time
	Value  T
	Raw    map[string]string // 받은 FID 전부. 표에 없는 것도 여기 남는다
	Err    error
}

// 아래는 internal/wstransport 의 타입을 외부에서 이름으로 쓰게 하는 별칭이다.
//
// 외부 모듈은 internal/ 을 import 하지 못한다. 1단계에서 연속조회 옵션이 이 문제로
// 막혔던 적이 있어(call.go 참고), WebSocket 쪽도 같은 방식으로 먼저 열어 둔다.
//
// 타입 별칭(=)이므로 errors.As 가 그대로 통한다 — 새 타입 정의가 아니다.
type (
	// GapError 는 끊겼다가 다시 붙은 구간이다. 그 사이 이벤트는 받지 못했다.
	GapError = wstransport.GapError
	// SlowConsumerError 는 구독 채널이 가득 차 이벤트를 버렸다는 뜻이다.
	SlowConsumerError = wstransport.SlowConsumerError
	// WSLoginError 는 WebSocket 로그인이 거부된 것이다.
	WSLoginError = wstransport.LoginError
	// WSAPIError 는 WebSocket 업무 오류(return_code != 0)다.
	WSAPIError = wstransport.APIError
)
