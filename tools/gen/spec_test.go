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
	got := gen.BuildTree(fields())

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
	got := gen.BuildTree(in)
	if len(got) != 2 || got[1].Element != "b" {
		t.Errorf("섹션 헤더를 버려야 한다: %v", names(got))
	}
}

func TestBuildTree_빈입력(t *testing.T) {
	if got := gen.BuildTree(nil); len(got) != 0 {
		t.Errorf("빈 입력 = %v", names(got))
	}
}

func names(ns []gen.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.Element
	}
	return out
}
