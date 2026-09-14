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
