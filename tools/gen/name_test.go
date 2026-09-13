package gen_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func TestGoName(t *testing.T) {
	cases := map[string]string{
		"get_domestic_stock_quote": "GetDomesticStockQuote",
		"get_domestic_elw_detail":  "GetDomesticElwDetail", // 도메인 약어는 올리지 않는다
		"bid_req_base_tm":          "BidReqBaseTm",
		"stk_cd":                   "StkCd",
		"sel_10th_pre_req_pre":     "Sel10thPreReqPre", // 숫자로 시작하는 조각
		"api_id":                   "APIID",            // 린트가 문제 삼는 것만 올린다
		"item_url":                 "ItemURL",
		"http_status":              "HTTPStatus",
		"":                         "",
	}
	for in, want := range cases {
		if got := gen.GoName(in); got != want {
			t.Errorf("GoName(%q) = %q, want %q", in, got, want)
		}
	}
}
