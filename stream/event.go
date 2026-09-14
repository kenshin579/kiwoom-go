// Package stream 은 실시간 이벤트 봉투를 담는다.
//
// 이 타입이 따로 사는 이유: 루트 kiwoom 패키지가 생성물 하위 패키지를 import 하므로
// (subclients.go), 하위 패키지가 루트를 import 할 수 없다. 봉투를 패키지마다 복사하면
// 모양이 같아도 Go 에서는 서로 다른 타입이 되어, 국내·미국 실시간을 한 핸들러로 받지
// 못한다. 아무것도 import 하지 않는 제3의 패키지를 두면 그 제약을 피하면서 정의가
// 하나로 남는다.
//
// 사용자는 kiwoom.Event 로 써도 된다 — 루트가 이 타입의 별칭을 내보낸다.
package stream

import "time"

// Event 는 실시간 한 건이다.
//
// Err 이 nil 이 아니면 Value 와 Raw 는 비어 있다 — 구멍(*kiwoom.GapError)이나
// 느린 소비자(*kiwoom.SlowConsumerError)를 알리는 봉투이기 때문이다.
type Event[T any] struct {
	Symbol string // 종목코드
	Name   string // 실시간 항목명
	Time   time.Time
	Value  T
	Raw    map[string]string // 받은 FID 전부. 표에 없는 것도 여기 남는다
	Err    error
}
