package kiwoom_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 통합 테스트가 들어 있는 파일. 가드는 이 파일의 import 를 본다.
const integrationFile = "integration_test.go"

// forbidden 은 통합 테스트가 **절대 import 하면 안 되는** 패키지다.
//
// 통합 테스트는 운영 서버를 친다. 모의투자가 아니다 — "스펙이 서버와 맞나" 의 정답을
// 운영 서버가 갖고 있고, 모의투자를 쓰면 "모의투자가 실시간을 주나" 라는 원래 묻지도
// 않은 질문이 하나 늘기 때문이다. 조회는 금전적 위험이 없다.
//
// 대신 안전의 근거가 "서버가 무해하다" 에서 **"코드가 주문에 닿을 수 없다"** 로 바뀐다.
// 그것을 약속이 아니라 여기서 구조로 막는다. 2단계에서 전송 계층의 재시도를 통째로
// 걷어낸 이유가 주문 이중 제출이었는데, 그것을 검증한다며 주문을 넣으면 앞뒤가 안 맞는다.
//
// 실시간 주문체결(00)·잔고(04)는 **받는** 채널이라 금지 대상이 아니다 — 체결을 알려줄 뿐
// 넣지 않는다. 그래서 domestic/realtime 은 여기 없다.
var forbidden = map[string]string{
	"github.com/kenshin579/kiwoom-go/domestic/order":       "국내주식 주문",
	"github.com/kenshin579/kiwoom-go/overseas/order":       "미국주식 주문",
	"github.com/kenshin579/kiwoom-go/domestic/creditorder": "국내주식 신용주문",
}

// TestIntegrationGuard_주문_패키지를_import_하지_않는다 는 통합 테스트가 주문 API 에
// 닿을 수 있게 되는 순간 실패한다.
//
// **빌드 태그가 없다 — 일부러 그렇다.** `//go:build integration` 을 붙이면 이 가드는
// 이미 실서버를 치기로 한 뒤에야 돌아, 정작 막아야 할 시점을 놓친다. `go test ./...` 가
// 늘 본다.
func TestIntegrationGuard_주문_패키지를_import_하지_않는다(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), integrationFile, nil, parser.ImportsOnly)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("%s 가 없다 — 통합 테스트를 지웠다면 이 가드도 함께 정리하라", integrationFile)
		}
		t.Fatalf("%s 파싱: %v", integrationFile, err)
	}

	var hits []string
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if what, bad := forbidden[path]; bad {
			hits = append(hits, path+" ("+what+")")
		}
	}
	sort.Strings(hits)

	if len(hits) > 0 {
		t.Errorf(`통합 테스트가 주문 패키지를 import 한다:
  %s

통합 테스트는 **운영 서버**를 친다. 읽기 전용만 쓴다 — 주문은 돈을 움직인다.
정말 주문 경로를 검증해야 한다면 모의투자 전용 별도 테스트로 분리하고,
이 가드의 forbidden 목록을 고치는 커밋을 따로 남겨라.`, strings.Join(hits, "\n  "))
	}
}

// TestIntegrationGuard_금지_목록이_실재하는_패키지다 는 목록이 오타로 조용히
// 비어 버리는 것을 막는다.
//
// 경로를 한 글자 틀리면 위 테스트는 **아무것도 걸러내지 못하면서 통과한다.**
// 이름표를 손으로 적는 곳마다 이 저장소가 붙여 온 규율과 같다(tools/gen/groups.go 참고).
func TestIntegrationGuard_금지_목록이_실재하는_패키지다(t *testing.T) {
	const prefix = "github.com/kenshin579/kiwoom-go/"
	for path := range forbidden {
		rel, ok := strings.CutPrefix(path, prefix)
		if !ok {
			t.Errorf("금지 목록의 경로가 이 모듈 것이 아니다: %q", path)
			continue
		}
		info, err := os.Stat(rel)
		if err != nil || !info.IsDir() {
			t.Errorf("금지 목록의 패키지가 없다: %q — 경로에 오타가 있으면 가드가 조용히 무력해진다", path)
		}
	}
}

// 통합 테스트가 실제로 부르는 메서드 이름에 주문이 섞이지 않았는지도 본다.
//
// import 가드만으로는 부족한 경우가 있다 — 조회 패키지 안에 주문성 API 가 섞여 들어오면
// import 는 깨끗한데 호출은 위험하다. 이름으로 한 겹 더 거른다.
var forbiddenCall = []string{"Order", "Buy", "Sell", "Cancel", "Modify", "Revoke"}

// TestIntegrationGuard_주문성_메서드를_부르지_않는다 는 이름에 주문 냄새가 나는 호출을 잡는다.
//
// 오탐이 날 수 있다(예: GetDomesticOrderBook 같은 조회). 그때는 목록을 줄이지 말고
// **그 호출이 정말 읽기 전용인지 확인한 뒤** 이 테스트에 예외를 명시적으로 적어라 —
// 예외를 적는 행위 자체가 한 번 더 묻는다.
var allowedCall = map[string]bool{
	// 여기에 예외를 적을 때는 왜 안전한지 한 줄 남길 것.
}

func TestIntegrationGuard_주문성_메서드를_부르지_않는다(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, integrationFile, nil, 0)
	if err != nil {
		t.Fatalf("%s 파싱: %v", integrationFile, err)
	}

	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name
		if allowedCall[name] {
			return true
		}
		for _, bad := range forbiddenCall {
			if strings.Contains(name, bad) {
				pos := fset.Position(call.Pos())
				hits = append(hits, name+" ("+integrationFile+":"+strconv.Itoa(pos.Line)+")")
				break
			}
		}
		return true
	})
	sort.Strings(hits)

	if len(hits) > 0 {
		t.Errorf(`통합 테스트가 주문성 이름의 메서드를 부른다:
  %s

읽기 전용인 것이 확실하면 allowedCall 에 이유와 함께 적어라.`, strings.Join(hits, "\n  "))
	}
}
