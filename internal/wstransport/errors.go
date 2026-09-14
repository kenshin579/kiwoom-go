// Package wstransport 는 키움 WebSocket 연결을 다룬다.
//
// REST 의 internal/transport 와 형제이고 토큰 발급기(internal/auth)를 공유한다.
// 연결은 경로마다 하나다 — 국내(/api/dostk/websocket)와 미국(/api/us/websocket).
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

func (e *LoginError) Error() string {
	return fmt.Sprintf("kiwoom: websocket 로그인 실패: %d %s", e.ReturnCode, e.ReturnMsg)
}

// GapError 는 끊겼다가 다시 붙은 구간이다. 그 사이 이벤트는 받지 못했다.
//
// 라이브러리가 대신 메꿔 주지 않는다 — 어떤 REST 호출로 메꿔야 하는지는 종목·타입마다
// 다르고, 우리가 고를 일이 아니다. 사용자가 이 신호를 보고 직접 정한다.
type GapError struct {
	Since time.Time // 끊긴 시각
	Until time.Time // 다시 붙은 시각
}

func (e *GapError) Error() string {
	return fmt.Sprintf("kiwoom: %s ~ %s 동안 실시간이 끊겼다 — 그 구간 이벤트는 받지 못했다",
		e.Since.Format(time.RFC3339), e.Until.Format(time.RFC3339))
}

// SlowConsumerError 는 구독 채널이 가득 차 이벤트를 버렸다는 뜻이다.
//
// 소켓 전체를 막지 않는다 — 한 구독의 느림이 다른 구독을 굶기면 안 된다.
type SlowConsumerError struct {
	Type    string // 실시간 항목 (0B 등)
	Item    string // 종목코드
	Dropped int    // 버린 건수
}

func (e *SlowConsumerError) Error() string {
	return fmt.Sprintf("kiwoom: %s/%s 구독이 느려 %d건을 버렸다 — 채널을 더 빨리 비우거나 버퍼를 늘려라",
		e.Type, e.Item, e.Dropped)
}
