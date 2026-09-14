package gen

import "strings"

// Kind 는 이 그룹을 어떤 템플릿으로 만드는지다.
//
// REST 와 WebSocket 은 전송 계층이 다르다 — 하위 클라이언트가 받는 것도 다르다.
type Kind string

const (
	KindREST      Kind = "rest"      // HTTP POST. 요청/응답
	KindRealtime  Kind = "realtime"  // WebSocket 구독. 채널을 준다
	KindCondition Kind = "condition" // WebSocket 요청/응답
)

// Group 은 카테고리 하나의 생성 대상이다.
type Group struct {
	Menu   string // 스펙의 "메뉴 위치" 앞 두 단계. 예: "국내주식 > 시세"
	Market string // "domestic" | "overseas"
	Pkg    string // 패키지 이름. 예: "quote"
	// 루트 Clients 의 필드 이름. 예: "DomesticQuote"
	//
	// 대문자 표기는 손으로 정한다 — GoName 의 규칙(_ 로 나눠 첫 글자만 대문자)을
	// 따르지 않는다. "DomesticELW"·"DomesticETF" 를 GoName 에 맡기면 "DomesticElw" 가 된다.
	Field  string
	Korean string // doc 주석용. 예: "국내주식 시세"
	Kind   Kind   // 생성 템플릿. 빈 값은 KindREST 로 본다
}

// Template 은 이 그룹의 Kind 를 돌려준다. 빈 값은 REST 다.
//
// 기존 25줄이 위치 기반 리터럴이라 Kind 자리에는 빈 문자열이 들어 있다 —
// 새 줄만 필드 이름으로 Kind 를 준다.
func (g Group) Template() Kind {
	if g.Kind == "" {
		return KindREST
	}
	return g.Kind
}

// TemplateName 은 같은 값을 string 으로 돌려준다.
//
// text/template 의 eq 는 **타입이 다르면 비교하지 못한다** — 템플릿 안의 "rest" 는
// string 이고 Kind 는 아니라서, .Template 로 비교하면 실행 시점에 터진다.
// 템플릿에서는 반드시 이쪽을 쓴다.
func (g Group) TemplateName() string { return string(g.Template()) }

// Groups 는 카테고리 → 패키지 표다.
//
// **여기만 지어낸 이름이다.** API 이름은 공식 영문명을 그대로 쓰지만(1단계 설계 §6.1)
// 패키지 이름은 스펙이 주지 않는다. `기관/외국인` 처럼 슬래시가 든 것도 있어 자동 변환은
// 불가능하다.
//
// 슬라이스로 두는 이유: 생성 순서가 맵 순회에 흔들리지 않아야 한다.
var Groups = []Group{
	{"국내주식 > 시세", "domestic", "quote", "DomesticQuote", "국내주식 시세", ""},
	{"국내주식 > 차트", "domestic", "chart", "DomesticChart", "국내주식 차트", ""},
	{"국내주식 > 종목정보", "domestic", "stock", "DomesticStock", "국내주식 종목정보", ""},
	{"국내주식 > 계좌", "domestic", "account", "DomesticAccount", "국내주식 계좌", ""},
	{"국내주식 > 순위정보", "domestic", "ranking", "DomesticRanking", "국내주식 순위정보", ""},
	{"국내주식 > ELW", "domestic", "elw", "DomesticELW", "국내주식 ELW", ""},
	{"국내주식 > ETF", "domestic", "etf", "DomesticETF", "국내주식 ETF", ""},
	{"국내주식 > 주문", "domestic", "order", "DomesticOrder", "국내주식 주문", ""},
	{"국내주식 > 업종", "domestic", "sector", "DomesticSector", "국내주식 업종", ""},
	{"국내주식 > 신용주문", "domestic", "creditorder", "DomesticCreditOrder", "국내주식 신용주문", ""},
	{"국내주식 > 대차거래", "domestic", "stocklending", "DomesticStockLending", "국내주식 대차거래", ""},
	{"국내주식 > 기관/외국인", "domestic", "investor", "DomesticInvestor", "국내주식 기관/외국인", ""},
	{"국내주식 > 테마", "domestic", "theme", "DomesticTheme", "국내주식 테마", ""},
	{"국내주식 > 관심종목", "domestic", "watchlist", "DomesticWatchlist", "국내주식 관심종목", ""},
	{"국내주식 > 공매도", "domestic", "shortsale", "DomesticShortSale", "국내주식 공매도", ""},

	{"미국주식 > 순위정보", "overseas", "ranking", "OverseasRanking", "미국주식 순위정보", ""},
	{"미국주식 > 종목정보", "overseas", "stock", "OverseasStock", "미국주식 종목정보", ""},
	{"미국주식 > 계좌", "overseas", "account", "OverseasAccount", "미국주식 계좌", ""},
	{"미국주식 > 차트", "overseas", "chart", "OverseasChart", "미국주식 차트", ""},
	{"미국주식 > 주문", "overseas", "order", "OverseasOrder", "미국주식 주문", ""},
	{"미국주식 > 시세", "overseas", "quote", "OverseasQuote", "미국주식 시세", ""},
	{"미국주식 > 환전", "overseas", "exchange", "OverseasExchange", "미국주식 환전", ""},
	{"미국주식 > 업종", "overseas", "sector", "OverseasSector", "미국주식 업종", ""},
	{"미국주식 > 관심종목", "overseas", "watchlist", "OverseasWatchlist", "미국주식 관심종목", ""},
	{"미국주식 > 투자정보", "overseas", "info", "OverseasInfo", "미국주식 투자정보", ""},

	// 3단계에서 더한 WebSocket 그룹 넷. 기존 25줄은 위치 기반 리터럴이라 Kind 자리가
	// 빈 문자열이고, 이 넷만 필드 이름을 써서 Kind 를 준다.
	{Menu: "국내주식 > 실시간시세", Market: "domestic", Pkg: "realtime", Field: "DomesticRealtime", Korean: "국내주식 실시간시세", Kind: KindRealtime},
	{Menu: "국내주식 > 조건검색", Market: "domestic", Pkg: "condition", Field: "DomesticCondition", Korean: "국내주식 조건검색", Kind: KindCondition},
	{Menu: "미국주식 > 실시간시세", Market: "overseas", Pkg: "realtime", Field: "OverseasRealtime", Korean: "미국주식 실시간시세", Kind: KindRealtime},
	{Menu: "미국주식 > 조건검색", Market: "overseas", Pkg: "condition", Field: "OverseasCondition", Korean: "미국주식 조건검색", Kind: KindCondition},
}

// skipped 는 **일부러** 생성하지 않는 카테고리다.
//
// OAuth 는 internal/auth 가 손으로 다룬다. 여기 적어 두는 이유는 Groups 에 없는
// 카테고리를 전부 에러로 잡기 위해서다 — 그래야 새 카테고리가 조용히 빠지지 않는다.
var skipped = map[string]bool{
	"OAuth 인증 > 접근토큰발급": true,
	"OAuth 인증 > 접근토큰폐기": true,
}

// menuKey 는 "국내주식 > 시세 > 주식호가요청(ka10004)" 에서 앞 두 단계만 뗀다.
func menuKey(menu string) string {
	parts := strings.SplitN(menu, " > ", 3)
	if len(parts) < 2 {
		return menu
	}
	return parts[0] + " > " + parts[1]
}

// Lookup 은 메뉴 위치로 생성 대상을 찾는다.
func Lookup(menu string) (Group, bool) {
	key := menuKey(menu)
	for _, g := range Groups {
		if g.Menu == key {
			return g, true
		}
	}
	return Group{}, false
}

// Skipped 는 일부러 생성하지 않는 카테고리인지 알려준다.
func Skipped(menu string) bool { return skipped[menuKey(menu)] }
