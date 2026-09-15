package gen_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func TestGroups_스펙의_모든_카테고리를_덮는다(t *testing.T) {
	spec, err := gen.LoadSpec("../spec/kiwoom_api_spec.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range spec.APIs {
		menu := api.Meta["메뉴 위치"]
		if menu == "" {
			t.Fatalf("메뉴 위치가 빈 API 가 있다: %v", api.Meta["API ID"])
		}
		if _, ok := gen.Lookup(menu); ok {
			continue
		}
		if gen.Skipped(menu) {
			continue
		}
		t.Errorf("표에도 제외 목록에도 없는 카테고리: %q (API %s)", menu, api.Meta["API ID"])
	}
}

func TestGroups_필드이름이_유일하다(t *testing.T) {
	seen := map[string]string{}
	for _, g := range gen.Groups {
		if prev, dup := seen[g.Field]; dup {
			t.Errorf("필드 이름 충돌: %q 와 %q 가 모두 %s", prev, g.Menu, g.Field)
		}
		seen[g.Field] = g.Menu
	}
	// 3단계에서 실시간 2 + 조건검색 2 = 4개가 늘어 25 → 29 가 됐다.
	if len(gen.Groups) != 29 {
		t.Errorf("카테고리 = %d개, want 29", len(gen.Groups))
	}
}

func TestGroups_시장과_패키지가_짝을_이룬다(t *testing.T) {
	for _, g := range gen.Groups {
		if g.Market != "domestic" && g.Market != "overseas" {
			t.Errorf("%s: Market = %q", g.Menu, g.Market)
		}
		if g.Pkg == "" || g.Field == "" || g.Korean == "" {
			t.Errorf("%s: 빈 값이 있다 %+v", g.Menu, g)
		}
	}
}
