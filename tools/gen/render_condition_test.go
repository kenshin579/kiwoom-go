package gen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestRenderCondition_trnm_으로_요청한다(t *testing.T) {
	src, err := RenderCondition(Target{
		Package:  "condition",
		GoName:   "ListDomesticConditionSearches",
		APIID:    "ka10171",
		APIName:  "조건검색 목록조회",
		MenuPath: "국내주식 > 조건검색 > 조건검색 목록조회(ka10171)",
		Path:     "/api/dostk/websocket",
		Request:  []Node{{Field: Field{Element: "trnm", Korean: "TR명", Type: "String", Required: "Y"}}},
		Response: []Node{{Field: Field{Element: "return_code", Korean: "결과코드", Type: "String"}}},
	}, "CNSRLST")
	if err != nil {
		t.Fatalf("RenderCondition: %v", err)
	}
	got := string(src)
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", got, parser.AllErrors); err != nil {
		t.Fatalf("생성물이 Go 로 파싱되지 않는다: %v\n%s", err, got)
	}
	for _, want := range []string{`"CNSRLST"`, "c.ws.Request(ctx", "ListDomesticConditionSearchesRequest"} {
		if !strings.Contains(got, want) {
			t.Errorf("생성물에 %q 가 없다:\n%s", want, got)
		}
	}
}
