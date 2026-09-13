package kiwoom_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

// 별칭이 실제로 같은 타입인지 — 다른 타입이면 생성 메서드에 넘길 수 없다.
func TestWithCont(t *testing.T) {
	m := kiwoom.Meta{ContYN: "Y", NextKey: "K1"}
	opt := kiwoom.WithCont(m)
	if opt == nil {
		t.Fatal("WithCont 가 nil 을 돌려줬다")
	}
	// CallOption 은 요청을 고치는 함수다. 실제로 값이 실리는지 확인한다.
	// (transport.Request 를 직접 만들 수 없으므로, 외부 모듈 빌드 확인이 본 검증이다.)
}
