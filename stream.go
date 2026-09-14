package kiwoom

import (
	"github.com/kenshin579/kiwoom-go/internal/wstransport"
	"github.com/kenshin579/kiwoom-go/stream"
)

// Event 는 실시간 한 건이다. stream.Event 의 별칭이다.
//
// 별칭이어야 한다 — 새 타입 정의면 생성물 하위 패키지가 주는 봉투와 다시 갈린다.
// 루트가 생성물을 import 하므로(subclients.go) 정의는 stream 패키지에 산다.
type Event[T any] = stream.Event[T]

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
