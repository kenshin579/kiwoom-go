package gen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestRenderRealtime_FID_를_json_태그로_쓴다(t *testing.T) {
	src, err := RenderRealtime(RealtimeTarget{
		Package: "realtime",
		GoName:  "SubscribeDomesticStockTrade",
		Type:    "0B",
		APIName: "주식체결",
		Fields: []RealtimeField{
			{FID: "10", Name: "CurrentPrice", Korean: "현재가"},
			{FID: "20", Name: "TradeTime", Korean: "체결시간"},
		},
	})
	if err != nil {
		t.Fatalf("RenderRealtime: %v", err)
	}
	got := string(src)

	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", got, parser.AllErrors); err != nil {
		t.Fatalf("생성물이 Go 로 파싱되지 않는다: %v\n%s", err, got)
	}
	for _, want := range []string{
		"`json:\"10\"`",
		"CurrentPrice string",
		"// 현재가",
		// Raw 자체는 Event 에 있다(client.go, wsSubClientTmpl). 여기서 볼 것은
		// 이 파일이 그것을 **채우는가** 다 — 모르는 FID 를 버리지 않는 성질이 거기 걸려 있다.
		`&ev.Raw`,
		`"0B"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("생성물에 %q 가 없다:\n%s", want, got)
		}
	}
}

func TestRenderRealtime_Raw_를_항상_채운다(t *testing.T) {
	src, err := RenderRealtime(RealtimeTarget{
		Package: "realtime", GoName: "SubscribeX", Type: "0X", APIName: "X",
		Fields: []RealtimeField{{FID: "1", Name: "A", Korean: "가"}},
	})
	if err != nil {
		t.Fatalf("RenderRealtime: %v", err)
	}
	// 모르는 FID 를 버리지 않으려면 원문을 맵으로도 한 번 더 풀어야 한다.
	if n := strings.Count(string(src), "json.Unmarshal(d.Values"); n != 2 {
		t.Errorf("values 를 %d번 푼다, 기대 2번(구조체 + Raw 맵)", n)
	}
}

// TestRenderRealtime_스펙의_설명을_주석에_싣는다 는 4번 결함의 회귀를 막는다.
//
// 버려지던 것이 장식이 아니다 — CumulativeTradeAmount 만 보고 "단위: 백만원" 인 줄 알
// 방법이 없고, 25(전일대비기호)의 코드 도메인도 설명 열에만 있다. 2단계 REST 생성물은
// 같은 것을 이미 싣고 있어 관례 위반이기도 하다.
func TestRenderRealtime_스펙의_설명을_주석에_싣는다(t *testing.T) {
	src, err := RenderRealtime(RealtimeTarget{
		Package: "realtime", GoName: "SubscribeDomesticStockTrade", Type: "0B", APIName: "주식체결",
		Fields: []RealtimeField{
			{FID: "14", Name: "CumulativeTradeAmount", Korean: "누적거래대금", Desc: "단위: 백만원"},
			// 개행이 든 설명. oneLine 을 거치지 않으면 둘째 줄이 주석 밖으로 새어
			// 생성물이 깨진다(1단계에서 실제로 깨졌다).
			{FID: "215", Name: "MarketOperationType", Korean: "장운영구분", Desc: "0 : 장시작전 알림,\n3 : 장시작"},
			{FID: "10", Name: "CurrentPrice", Korean: "현재가", Required: "Y", Length: "20"},
		},
	})
	if err != nil {
		t.Fatalf("RenderRealtime: %v", err)
	}
	got := string(src)
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", got, parser.AllErrors); err != nil {
		t.Fatalf("생성물이 Go 로 파싱되지 않는다: %v\n%s", err, got)
	}
	for _, want := range []string{
		"// 누적거래대금 (단위: 백만원)",
		"// 장운영구분 (0 : 장시작전 알림, 3 : 장시작)", // 한 줄로 접혔다
		"// 현재가 (필수, 20자)",                // REST 의 comment() 와 같은 모양
	} {
		if !strings.Contains(got, want) {
			t.Errorf("생성물에 %q 가 없다:\n%s", want, got)
		}
	}
}
