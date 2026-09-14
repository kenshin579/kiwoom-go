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

// TestConditionTrnm_스펙에서_뽑은_값이_설계표와_같다 는 조건검색 8건의 trnm 을 전부 대조한다.
//
// 표는 설계 §2 에서 온다. 여기 적어 두는 이유: trnm 을 틀리게 뽑으면 **생성 시점에는
// 조용히 지나가고** 실서버에서 *kiwoom.WSAPIError 로만 드러난다. 생성물을 훑어봐도
// 대문자 일곱 글자라 눈에 안 띈다.
//
// 스펙 파일에서 찾아 대조하므로 스펙이 바뀌어도 여기서 잡힌다 — 8건이 다 나오지 않으면
// 그것도 실패다(조건검색 API 가 스펙에서 사라지거나 메뉴가 바뀐 경우).
func TestConditionTrnm_스펙에서_뽑은_값이_설계표와_같다(t *testing.T) {
	want := map[string]string{
		"ka10171":  "CNSRLST",
		"ka10172":  "CNSRREQ",
		"ka10173":  "CNSRREQ",
		"ka10174":  "CNSRCLR",
		"usa20280": "GCNSRLST",
		"usa20281": "GCNSRREQ",
		"usa20290": "GCNSRREQ",
		"usa20291": "GCNSRCLR",
	}

	spec, err := gen.LoadSpec(filepath.Join(fidSpecDir, "kiwoom_api_spec.json"))
	if err != nil {
		t.Fatalf("스펙 읽기: %v", err)
	}

	seen := map[string]bool{}
	for _, api := range spec.APIs {
		id := api.Meta["API ID"]
		exp, ok := want[id]
		if !ok {
			continue
		}
		seen[id] = true
		got, err := gen.ConditionTrnm(api)
		if err != nil {
			t.Errorf("%s: ConditionTrnm: %v", id, err)
			continue
		}
		if got != exp {
			t.Errorf("%s 의 trnm = %q, 설계 §2 의 표는 %q", id, got, exp)
		}
	}

	var absent []string
	for id := range want {
		if !seen[id] {
			absent = append(absent, id)
		}
	}
	sort.Strings(absent)
	if len(absent) > 0 {
		t.Errorf("스펙에서 찾지 못한 조건검색 API %v — 대조가 비어 있었다", absent)
	}
}

// TestConditionTrnm_요청에_trnm_이_없으면_에러다 는 REST API 를 넣어 본다.
//
// 조용히 빈 문자열을 돌려주면 trnm 이 빠진 요청이 생성돼 서버가 거절한다.
func TestConditionTrnm_요청에_trnm_이_없으면_에러다(t *testing.T) {
	api := gen.API{Request: gen.Body{Body: []gen.Field{{Element: "stk_cd"}}}}
	if _, err := gen.ConditionTrnm(api); err == nil {
		t.Error("trnm 이 없는데 에러가 아니다")
	}
}
