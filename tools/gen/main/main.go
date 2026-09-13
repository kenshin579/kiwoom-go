// Command main 은 벤더링한 스펙에서 kiwoom-go 클라이언트 코드를 생성한다.
//
//	cd tools && go run ./gen/main
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

// groups 는 이번 단계에서 생성할 카테고리와 그 출력 패키지다.
var groups = map[string]string{
	"국내주식 > 시세":   "quote",
	"국내주식 > 차트":   "chart",
	"국내주식 > 종목정보": "stock",
}

func main() {
	spec, err := gen.LoadSpec("spec/kiwoom_api_spec.json")
	if err != nil {
		log.Fatalf("스펙 읽기: %v", err)
	}
	names, err := gen.LoadNames("spec/api_names.json")
	if err != nil {
		log.Fatalf("이름표 읽기: %v", err)
	}

	counts := map[string]int{}
	var missing []string

	// 맵 순회 순서가 달라도 결과가 같도록 키를 정렬한다.
	keys := make([]string, 0, len(spec.APIs))
	for k := range spec.APIs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		api := spec.APIs[k]
		menu := api.Meta["메뉴 위치"]
		pkg := ""
		for prefix, p := range groups {
			if strings.HasPrefix(menu, prefix) {
				pkg = p
				break
			}
		}
		if pkg == "" {
			continue
		}

		id := api.Meta["API ID"]
		name, ok := names[id]
		if !ok {
			missing = append(missing, id)
			continue
		}

		reqTree, err := gen.BuildTree(api.Request.Body)
		if err != nil {
			log.Fatalf("%s(%s) 요청 트리: %v", id, menu, err)
		}
		resTree, err := gen.BuildTree(api.Response.Body)
		if err != nil {
			log.Fatalf("%s(%s) 응답 트리: %v", id, menu, err)
		}

		target := gen.Target{
			Package:  pkg,
			GoName:   gen.GoName(name),
			APIID:    id,
			APIName:  api.Meta["API 명"],
			MenuPath: menu,
			Path:     api.Meta["URL"],
			Request:  reqTree,
			Response: resTree,
		}
		src, err := gen.Render(target)
		if err != nil {
			log.Fatalf("%s 렌더: %v", id, err)
		}
		out := filepath.Join("..", "domestic", pkg, name+".go")
		if err := os.WriteFile(out, src, 0o644); err != nil {
			log.Fatalf("%s 쓰기: %v", out, err)
		}
		counts[pkg]++
	}

	for _, p := range []string{"quote", "chart", "stock"} {
		fmt.Printf("%s: %d개\n", p, counts[p])
	}
	if len(missing) > 0 {
		// 이름표가 없으면 멈춘다. api-id 로 대충 이름을 지으면 나중에 바꿀 수 없다.
		log.Fatalf("이름표 없는 API %d개: %v", len(missing), missing)
	}
}
