package kiwoom_test

import (
	"errors"
	"testing"
	"time"

	kiwoom "github.com/kenshin579/kiwoom-go"
	drt "github.com/kenshin579/kiwoom-go/domestic/realtime"
	ort "github.com/kenshin579/kiwoom-go/overseas/realtime"
	"github.com/kenshin579/kiwoom-go/stream"
)

// TestGapError_는_errors_As_로_잡힌다 는 외부에서 구멍을 구분할 수 있는지 본다.
//
// 이걸 못 하면 사용자는 끊김과 다른 에러를 가르지 못하고, 결국 구멍을 무시하게 된다.
func TestGapError_는_errors_As_로_잡힌다(t *testing.T) {
	since := time.Now().Add(-time.Minute)
	until := time.Now()
	var err error = &kiwoom.GapError{Since: since, Until: until}

	var gap *kiwoom.GapError
	if !errors.As(err, &gap) {
		t.Fatal("errors.As 가 *kiwoom.GapError 를 잡지 못한다")
	}
	if !gap.Since.Equal(since) || !gap.Until.Equal(until) {
		t.Errorf("시각이 바뀌었다: %+v", gap)
	}
}

// TestSlowConsumerError_는_errors_As_로_잡힌다 도 같은 이유다.
func TestSlowConsumerError_는_errors_As_로_잡힌다(t *testing.T) {
	var err error = &kiwoom.SlowConsumerError{Type: "0B", Item: "005930", Dropped: 7}
	var slow *kiwoom.SlowConsumerError
	if !errors.As(err, &slow) {
		t.Fatal("errors.As 가 *kiwoom.SlowConsumerError 를 잡지 못한다")
	}
	if slow.Dropped != 7 {
		t.Errorf("Dropped = %d, 기대 7", slow.Dropped)
	}
}

// TestReconnectingError_는_errors_As_로_잡힌다 는 셋째 신호도 외부에서 갈리는지 본다.
//
// 이것이 GapError 와 갈리지 않으면 "끊겼다 붙었다" 와 "한 시간째 못 붙고 있다" 가
// 한 덩어리가 된다 — 구독자가 장애를 조용한 장세로 착각하는 자리다.
func TestReconnectingError_는_errors_As_로_잡힌다(t *testing.T) {
	cause := errors.New("dial 실패")
	var err error = &kiwoom.ReconnectingError{Since: time.Now(), Attempts: 3, Last: cause}

	var rec *kiwoom.ReconnectingError
	if !errors.As(err, &rec) {
		t.Fatal("errors.As 가 *kiwoom.ReconnectingError 를 잡지 못한다")
	}
	if rec.Attempts != 3 {
		t.Errorf("Attempts = %d, 기대 3", rec.Attempts)
	}
	// Unwrap 이 원인을 내주므로 errors.Is 로도 가려야 한다.
	if !errors.Is(err, cause) {
		t.Error("errors.Is 가 원인을 찾지 못한다 — Unwrap 이 끊겼다")
	}
	// 구멍과 섞이면 안 된다.
	var gap *kiwoom.GapError
	if errors.As(err, &gap) {
		t.Error("ReconnectingError 가 GapError 로도 잡힌다")
	}
}

// handleAny 는 국내·미국 실시간을 한 핸들러로 받는 사용자 코드를 흉내낸다.
func handleAny[T any](ev kiwoom.Event[T]) string { return ev.Symbol + "/" + ev.Name }

// TestEvent_국내와_미국을_한_핸들러로_받는다 는 stream 패키지를 따로 둔 이유를 지킨다.
//
// 봉투를 패키지마다 복사하면 모양이 같아도 Go 에서는 서로 다른 타입이 되어, 이 함수를
// 두 벌 쓰게 된다. 그 퇴행은 컴파일이 되는 채로 들어오므로 테스트로 못 박는다.
func TestEvent_국내와_미국을_한_핸들러로_받는다(t *testing.T) {
	dom := kiwoom.Event[drt.DomesticStockTrade]{
		Symbol: "005930", Name: "주식체결",
		Value: drt.DomesticStockTrade{CurrentPrice: "+71000"},
		Raw:   map[string]string{"10": "+71000"},
	}
	ovs := kiwoom.Event[ort.OverseasTradePrice]{
		Symbol: "AAPL", Name: "체결가",
		Value: ort.OverseasTradePrice{CurrentPrice: "231.4"},
		Raw:   map[string]string{"10": "231.4"},
	}

	if got, want := handleAny(dom), "005930/주식체결"; got != want {
		t.Errorf("국내 = %q, 기대 %q", got, want)
	}
	if got, want := handleAny(ovs), "AAPL/체결가"; got != want {
		t.Errorf("미국 = %q, 기대 %q", got, want)
	}
	// Raw 는 표에 없는 FID 도 담는다 — 구조체 필드와 같은 값이 보여야 한다.
	if dom.Raw["10"] != dom.Value.CurrentPrice {
		t.Errorf("Raw[10] = %q, Value.CurrentPrice = %q", dom.Raw["10"], dom.Value.CurrentPrice)
	}
}

// TestEvent_는_stream_Event_의_별칭이다 는 루트 별칭이 새 타입 정의로 바뀌는 퇴행을 잡는다.
//
// 새 타입이 되면 생성물이 주는 채널을 kiwoom.Event 로 받지 못한다 — 사용자가 첫 줄에서 막힌다.
func TestEvent_는_stream_Event_의_별칭이다(t *testing.T) {
	var se stream.Event[drt.DomesticStockTrade]
	var ke kiwoom.Event[drt.DomesticStockTrade] = se // 별칭이 아니면 컴파일되지 않는다
	_ = ke
}
