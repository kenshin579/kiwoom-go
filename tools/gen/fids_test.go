package gen_test

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

const fidSpecDir = "../spec"

// TestFids_스펙의_모든_FID_가_표에_있다 는 이름표가 빠진 FID 를 잡는다.
//
// groups.go 의 카테고리 완전성 테스트와 같은 성격이다 — 빠진 것이 조용히 넘어가면
// 생성물에서 필드 하나가 통째로 사라진다.
func TestFids_스펙의_모든_FID_가_표에_있다(t *testing.T) {
	spec, err := gen.LoadSpec(filepath.Join(fidSpecDir, "kiwoom_api_spec.json"))
	if err != nil {
		t.Fatalf("스펙 읽기: %v", err)
	}

	want := map[string]string{} // fid → 한글명
	for _, api := range spec.APIs {
		menu := api.Meta["메뉴 위치"]
		if !strings.Contains(menu, "실시간시세") && !strings.Contains(menu, "조건검색") {
			continue
		}
		for _, f := range api.Response.Body {
			if f.Depth == 2 && isDigits(f.Element) {
				want[f.Element] = f.Korean
			}
		}
	}

	var missing []string
	for fid := range want {
		if _, ok := gen.LookupFid(fid); !ok {
			missing = append(missing, fid+"("+want[fid]+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("표에 없는 FID %d개 — tools/gen/fids.go 에 넣어라:\n  %v", len(missing), missing)
	}
}

// TestFids_이름이_겹치지_않는다 는 같은 Go 이름을 두 FID 에 준 것을 잡는다.
//
// 겹치면 같은 구조체에 같은 필드가 두 번 선언돼 컴파일이 깨지지만, 어느 FID 때문인지
// 생성물 쪽 에러로는 알기 어렵다. 표에서 잡는 편이 훨씬 빠르다.
func TestFids_이름이_겹치지_않는다(t *testing.T) {
	byName := map[string][]string{}
	for _, f := range gen.Fids {
		byName[f.Name] = append(byName[f.Name], f.FID)
	}
	for name, fids := range byName {
		if len(fids) > 1 {
			sort.Strings(fids)
			t.Errorf("Go 이름 %q 를 FID %v 가 함께 쓴다", name, fids)
		}
	}
}

var goIdent = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)

// TestFids_이름이_Go_식별자다 는 소문자 시작·기호·빈 이름을 잡는다.
func TestFids_이름이_Go_식별자다(t *testing.T) {
	for _, f := range gen.Fids {
		if !goIdent.MatchString(f.Name) {
			t.Errorf("FID %s 의 이름 %q 는 내보낼 수 있는 Go 식별자가 아니다", f.FID, f.Name)
		}
		if f.Korean == "" {
			t.Errorf("FID %s 에 한글명이 없다 — 사람이 표를 훑을 수 없다", f.FID)
		}
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
