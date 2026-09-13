package gen_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func fields() []gen.Field {
	return []gen.Field{
		{Element: "bid_req_base_tm", Depth: 0, Korean: "호가잔량기준시간", Type: "String"},
		{Element: "lvl1", Depth: 0, Korean: "1단계목록", Type: "LIST"},
		{Element: "dt", Depth: 1, Korean: "일자", Type: "String"},
		{Element: "lvl2", Depth: 1, Korean: "2단계목록", Type: "LIST"},
		{Element: "deep", Depth: 2, Korean: "깊은값", Type: "String"},
		{Element: "after", Depth: 0, Korean: "뒤에오는값", Type: "String"},
	}
}

func TestBuildTree_중첩을_3단계까지_만든다(t *testing.T) {
	got, err := gen.BuildTree(fields())
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("최상위 노드 = %d, want 3 (%v)", len(got), names(got))
	}
	if got[0].Element != "bid_req_base_tm" || len(got[0].Children) != 0 {
		t.Errorf("0번 = %+v", got[0])
	}

	lvl1 := got[1]
	if lvl1.Element != "lvl1" || !lvl1.IsList {
		t.Fatalf("1번이 LIST 여야 한다: %+v", lvl1)
	}
	if len(lvl1.Children) != 2 {
		t.Fatalf("lvl1 자식 = %d, want 2 (%v)", len(lvl1.Children), names(lvl1.Children))
	}
	lvl2 := lvl1.Children[1]
	if lvl2.Element != "lvl2" || !lvl2.IsList || len(lvl2.Children) != 1 {
		t.Fatalf("lvl2 = %+v", lvl2)
	}
	if lvl2.Children[0].Element != "deep" {
		t.Errorf("deep 이 lvl2 아래여야 한다: %+v", lvl2.Children[0])
	}

	if got[2].Element != "after" {
		t.Errorf("깊이가 0 으로 돌아오면 최상위다: %+v", got[2])
	}
}

func TestBuildTree_섹션은_버린다(t *testing.T) {
	in := []gen.Field{
		{Element: "a", Depth: 0, Type: "String"},
		{Element: "sect", Depth: 0, Type: "", IsSection: true},
		{Element: "b", Depth: 0, Type: "String"},
	}
	got, err := gen.BuildTree(in)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if len(got) != 2 || got[1].Element != "b" {
		t.Errorf("섹션 헤더를 버려야 한다: %v", names(got))
	}
}

func TestBuildTree_빈입력(t *testing.T) {
	got, err := gen.BuildTree(nil)
	if err != nil {
		t.Fatalf("BuildTree: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("빈 입력 = %v", names(got))
	}
}

func TestBuildTree_LIST가_아닌데_자식이_있으면_에러다(t *testing.T) {
	// ka10173(조건검색 실시간)이 실제로 이런 모양이다 — Object/List<Map> 가 자식을 갖는다.
	// 예전에는 자식들을 최상위로 끌어올려 조용히 구조를 뭉갰다.
	in := []gen.Field{
		{Element: "data", Depth: 0, Type: "List<Map>"},
		{Element: "name", Depth: 1, Type: "String"},
	}
	if _, err := gen.BuildTree(in); err == nil {
		t.Fatal("에러여야 한다 — 조용히 끌어올리면 구조가 틀린 채 생성된다")
	}
}

func TestBuildTree_진짜_깊이건너뛰기도_에러다(t *testing.T) {
	// depth 0 다음에 바로 depth 2. 스펙 전체에 0건이라 막아도 잃을 게 없다.
	in := []gen.Field{
		{Element: "a", Depth: 0, Type: "String"},
		{Element: "b", Depth: 2, Type: "String"},
	}
	if _, err := gen.BuildTree(in); err == nil {
		t.Fatal("에러여야 한다")
	}
}

func names(ns []gen.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Element
	}
	return out
}
