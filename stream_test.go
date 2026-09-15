package kiwoom_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	kiwoom "github.com/kenshin579/kiwoom-go"
	domcond "github.com/kenshin579/kiwoom-go/domestic/condition"
	drt "github.com/kenshin579/kiwoom-go/domestic/realtime"
	ovscond "github.com/kenshin579/kiwoom-go/overseas/condition"
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

// **Important 3 을 못 박는다.** 국내·미국 조건검색 푸시의 값 구조체는 글자까지 같아야 한다.
//
// 두 API 의 푸시는 **같은 다섯 FID** 를 싣는다(841·9001·843·20·907). 같은 FID 는 어디서나
// 같은 낱말이라는 것이 이 저장소의 규율이고, 그 낱말은 tools/gen/fids.go 의 표에서 온다 —
// 미국 쪽 이름을 지어낸 것이 아니라 이미 가진 이름을 쓴 것이라는 뜻이다. 한쪽만 고치면
// 그 규율이 조용히 깨지므로 여기서 잡는다.
func TestConditionMatch_국내와_미국이_글자까지_같다(t *testing.T) {
	dom := reflect.TypeOf(domcond.DomesticRealtimeConditionMatch{})
	ovs := reflect.TypeOf(ovscond.OverseasRealtimeConditionMatch{})

	if dom.NumField() != ovs.NumField() {
		t.Fatalf("필드 수 = 국내 %d, 미국 %d", dom.NumField(), ovs.NumField())
	}
	if dom.NumField() != 5 {
		t.Fatalf("필드 수 = %d, 기대 5 (841·9001·843·20·907)", dom.NumField())
	}
	for i := 0; i < dom.NumField(); i++ {
		d, o := dom.Field(i), ovs.Field(i)
		if d.Name != o.Name || d.Type != o.Type || d.Tag != o.Tag {
			t.Errorf("%d번 필드가 갈렸다: 국내 %s %s `%s` / 미국 %s %s `%s`",
				i, d.Name, d.Type, d.Tag, o.Name, o.Type, o.Tag)
		}
	}

	// FID 표의 이름 그대로여야 한다. 표를 고치면 여기도 함께 움직여야 한다.
	want := map[string]string{
		"841":  "SequenceNumber",
		"9001": "StockOrSectorCode",
		"843":  "InsertDeleteType",
		"20":   "TradeTime",
		"907":  "TradeSide",
	}
	for i := 0; i < ovs.NumField(); i++ {
		f := ovs.Field(i)
		fid := f.Tag.Get("json")
		if name, ok := want[fid]; !ok {
			t.Errorf("표에 없는 FID %q 가 들어왔다", fid)
		} else if f.Name != name {
			t.Errorf("FID %s 의 이름 = %s, 기대 %s (tools/gen/fids.go 의 표)", fid, f.Name, name)
		}
	}
}

// 푸시 봉투의 거래소구분은 값 구조체가 아니라 봉투에 산다.
//
// values 밖에 붙는 필드라 FID 표에 자리가 없다. Raw 에 섞으면 "Raw 의 열쇠는 FID 숫자"
// 라는 약속이, 값 구조체에 넣으면 "필드 이름은 FID 표에서 온다" 는 약속이 깨진다.
func TestEvent_거래소구분은_봉투에_있다(t *testing.T) {
	ev := kiwoom.Event[ovscond.OverseasRealtimeConditionMatch]{Symbol: "COIN", StexTp: "ND"}
	if ev.StexTp != "ND" {
		t.Errorf("StexTp = %q, 기대 ND", ev.StexTp)
	}
	if _, ok := reflect.TypeOf(ovscond.OverseasRealtimeConditionMatch{}).FieldByName("StexTp"); ok {
		t.Error("값 구조체에 StexTp 가 들어갔다 — 그 자리의 이름은 FID 표에서 와야 한다")
	}
}

// 조건검색 푸시가 갈 곳을 찾지 못한 것도 **외부에서 갈려야** 한다.
//
// 이것이 없으면 사용자는 그 상황을 "조건에 걸린 종목이 없다" 와 구분할 수 없다 —
// 라이브러리에서 구멍이 소리 없이 닫히던 유일한 자리였다.
func TestUnroutedConditionPushError_는_errors_As_로_잡힌다(t *testing.T) {
	var err error = &kiwoom.UnroutedConditionPushError{Type: "S2", Name: "조건검색", Item: "COIN", Seq: "2"}

	var ue *kiwoom.UnroutedConditionPushError
	if !errors.As(err, &ue) {
		t.Fatal("errors.As 가 *kiwoom.UnroutedConditionPushError 를 잡지 못한다")
	}
	if ue.Seq != "2" || ue.Item != "COIN" {
		t.Errorf("= %+v, 기대 2/COIN", ue)
	}
	// 다른 신호와 섞이면 안 된다. 섞이면 "끊겼다" 와 "갈 곳이 없다" 가 한 덩어리가 된다.
	var gap *kiwoom.GapError
	if errors.As(err, &gap) {
		t.Error("UnroutedConditionPushError 가 GapError 로도 잡힌다")
	}
	var slow *kiwoom.SlowConsumerError
	if errors.As(err, &slow) {
		t.Error("UnroutedConditionPushError 가 SlowConsumerError 로도 잡힌다")
	}
}

// 조건검색의 구멍과 실시간의 구멍은 **뜻이 다르다.** 값으로 갈리는지 외부에서 본다.
func TestGapError_조건검색과_실시간이_값으로_갈린다(t *testing.T) {
	realtime := &kiwoom.GapError{Since: time.Now(), Until: time.Now(), Resubscribed: true}
	cond := &kiwoom.GapError{Since: time.Now(), Until: time.Now()}

	if !realtime.Resubscribed {
		t.Error("실시간 구멍의 Resubscribed 가 false 다")
	}
	if cond.Resubscribed {
		t.Error("조건검색 구멍의 Resubscribed 가 true 다 — 되살릴 REG 가 없는 등록이다")
	}
	if realtime.Error() == cond.Error() {
		t.Error("문장이 같다 — ev.Err 하나로 처리하는 핸들러가 둘을 구분하지 못한다")
	}
}
