package quote_test

import (
	"reflect"
	"testing"

	"github.com/kenshin579/kiwoom-go/domestic/quote"
)

// 생성물이 실제로 쓸 수 있는 모양인지 확인한다.
// 스펙 전체를 다시 검증하지는 않는다 — 그건 생성기 테스트의 몫이다.
func TestGeneratedShape(t *testing.T) {
	var req quote.GetDomesticStockQuoteRequest
	rt := reflect.TypeOf(req)
	f, ok := rt.FieldByName("StkCd")
	if !ok {
		t.Fatal("StkCd 필드가 없다")
	}
	if got := f.Tag.Get("json"); got != "stk_cd" {
		t.Errorf("json 태그 = %q, want stk_cd", got)
	}
	if f.Type.Kind() != reflect.String {
		t.Errorf("필드 타입 = %v, want string (모든 필드는 문자열이다)", f.Type.Kind())
	}

	var resp quote.GetDomesticStockQuoteResponse
	if reflect.TypeOf(resp).NumField() == 0 {
		t.Error("응답 필드가 비어 있다")
	}
}
