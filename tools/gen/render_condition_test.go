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

// TestRenderCondition_요청_구조체에_trnm_이_없다 는 1번 결함의 회귀를 막는다.
//
// 요청 구조체에 trnm 을 두면 호출자가 안 채우고 보낼 수 있고, 그러면 {"trnm":""} 이 나가
// 서버 응답이 짝을 찾지 못한다 — 에러가 아니라 ctx 만료까지 이어지는 조용한 행이다.
// 값은 전송 직전에 익명 래퍼가 넣는다.
func TestRenderCondition_요청_구조체에_trnm_이_없다(t *testing.T) {
	src, err := RenderCondition(Target{
		Package: "condition",
		GoName:  "ListDomesticConditionSearches",
		APIID:   "ka10171",
		Request: []Node{
			{Field: Field{Element: "trnm", Korean: "TR명", Required: "Y", Description: "CNSRLST고정값"}},
			{Field: Field{Element: "seq", Korean: "일련번호"}},
		},
	}, "CNSRLST")
	if err != nil {
		t.Fatalf("RenderCondition: %v", err)
	}
	got := string(src)

	reqType := got[strings.Index(got, "type ListDomesticConditionSearchesRequest struct {"):]
	reqType = reqType[:strings.Index(reqType, "\n}")]
	if strings.Contains(reqType, "Trnm") {
		t.Errorf("요청 구조체에 Trnm 이 남아 있다 — 호출자가 비워 둘 수 있게 된다:\n%s", reqType)
	}
	if !strings.Contains(reqType, "Seq string") {
		t.Errorf("trnm 말고 다른 필드까지 사라졌다:\n%s", reqType)
	}
	// 래퍼가 값을 넣는지 본다. 호출자가 고를 수 없어야 한다.
	for _, want := range []string{
		"Trnm string `json:\"trnm\"`",
		`}{Trnm: "CNSRLST", ListDomesticConditionSearchesRequest: req}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("전송 직전에 trnm 을 넣는 코드에 %q 가 없다:\n%s", want, got)
		}
	}
}

// TestRenderCondition_숫자_element_는_FID_표를_거친다 는 2번 결함의 회귀를 막는다.
//
// 조건검색 결과 행은 depth 1 이라 실시간 갈래(RealtimeFields)를 타지 않는다. 그래서
// 표를 안 거치면 N9001·N318 같은 이름이 공개 표면으로 새어 나간다(설계 §6).
func TestRenderCondition_숫자_element_는_FID_표를_거친다(t *testing.T) {
	src, err := RenderCondition(Target{
		Package: "condition", GoName: "X", APIID: "ka1",
		Request: []Node{{Field: Field{Element: "trnm", Description: "CNSRREQ 고정값"}}},
		Response: []Node{
			{Field: Field{Element: "data", Type: "LIST"}, IsList: true, Children: []Node{
				{Field: Field{Element: "9001", Korean: "종목코드"}},
				{Field: Field{Element: "318", Korean: "소업종"}},
				{Field: Field{Element: "stex_tp", Korean: "거래소구분"}},
			}},
		},
	}, "CNSRREQ")
	if err != nil {
		t.Fatalf("RenderCondition: %v", err)
	}
	got := string(src)
	for _, want := range []string{
		"StockOrSectorCode string `json:\"9001\"`",
		"SubSector string `json:\"318\"`",
		"StexTp string `json:\"stex_tp\"`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("생성물에 %q 가 없다:\n%s", want, got)
		}
	}
	if strings.Contains(got, "N9001") || strings.Contains(got, "N318") {
		t.Errorf("FID 이름이 그대로 새어 나왔다:\n%s", got)
	}
}

// TestRenderCondition_표에_없는_FID_면_멈춘다 는 조용한 통과를 막는다.
//
// 다른 곳과 같은 규율이다 — RealtimeFields 도 표에 없으면 에러를 낸다.
func TestRenderCondition_표에_없는_FID_면_멈춘다(t *testing.T) {
	_, err := RenderCondition(Target{
		Package: "condition", GoName: "X", APIID: "ka1",
		Request: []Node{{Field: Field{Element: "trnm", Description: "CNSRREQ 고정값"}}},
		Response: []Node{
			{Field: Field{Element: "data", Type: "LIST"}, IsList: true, Children: []Node{
				{Field: Field{Element: "999999", Korean: "없는FID"}},
			}},
		},
	}, "CNSRREQ")
	if err == nil {
		t.Fatal("표에 없는 FID 인데 에러가 아니다")
	}
	if !strings.Contains(err.Error(), "999999") {
		t.Errorf("에러가 어느 FID 인지 말하지 않는다: %v", err)
	}
}
