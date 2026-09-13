package gen

import "strings"

// initialisms 는 Go 린트가 문제 삼는 약어만 담는다.
//
// elw·etf 같은 도메인 약어는 일부러 넣지 않는다 — 목록을 키우면 "예외 없는 규칙" 이라는
// 이 생성기의 전제가 무너지고, 사용자가 문서에서 본 이름을 그대로 추측하지 못하게 된다.
var initialisms = map[string]string{
	"api":  "API",
	"id":   "ID",
	"url":  "URL",
	"http": "HTTP",
}

// GoName 은 snake_case 를 Go 식별자로 바꾼다.
//
// "250hgst" 처럼 요소명 전체가 숫자로 시작하면(예: 250일 최고가) 그대로는 식별자로
// 쓸 수 없다 — Go 식별자는 숫자로 시작하지 못한다. 이때만 "N" 을 앞에 붙인다.
// "sel_10th_..." 처럼 숫자가 두 번째 이상 조각에 있는 경우는 앞 조각이 이미 글자로
// 시작하므로 해당하지 않는다.
func GoName(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		if up, ok := initialisms[part]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	name := b.String()
	if name != "" && name[0] >= '0' && name[0] <= '9' {
		name = "N" + name
	}
	return name
}
