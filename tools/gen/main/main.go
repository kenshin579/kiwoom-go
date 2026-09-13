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

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func main() {
	spec, err := gen.LoadSpec("spec/kiwoom_api_spec.json")
	if err != nil {
		log.Fatalf("스펙 읽기: %v", err)
	}
	names, err := gen.LoadNames("spec/api_names.json")
	if err != nil {
		log.Fatalf("이름표 읽기: %v", err)
	}

	// 패키지 디렉터리를 먼저 만들고 client.go 를 찍는다.
	for _, g := range gen.Groups {
		dir := filepath.Join("..", g.Market, g.Pkg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("%s 만들기: %v", dir, err)
		}
		src, err := gen.RenderSubClient(g)
		if err != nil {
			log.Fatalf("%s client.go 렌더: %v", g.Pkg, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "client.go"), src, 0o644); err != nil {
			log.Fatalf("%s client.go 쓰기: %v", dir, err)
		}
	}

	// 루트 배선.
	src, err := gen.RenderClients(gen.Groups)
	if err != nil {
		log.Fatalf("subclients 렌더: %v", err)
	}
	if err := os.WriteFile(filepath.Join("..", "subclients.go"), src, 0o644); err != nil {
		log.Fatalf("subclients.go 쓰기: %v", err)
	}

	counts := map[string]int{}
	seen := map[string]string{}
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
		id := api.Meta["API ID"]

		g, ok := gen.Lookup(menu)
		if !ok {
			if gen.Skipped(menu) {
				continue
			}
			// 표에도 제외 목록에도 없으면 멈춘다 — 새 카테고리가 조용히 빠지면 안 된다.
			log.Fatalf("표에 없는 카테고리: %q (API %s). tools/gen/groups.go 에 넣거나 제외 목록에 적어라", menu, id)
		}

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

		out := filepath.Join("..", g.Market, g.Pkg, name+".go")
		// 이름이 겹치면 조용히 덮어쓴다 — 이름표 없음은 멈추면서 중복은 안 멈추면
		// API 하나가 소리 없이 사라진다.
		if prev, dup := seen[out]; dup {
			log.Fatalf("이름 충돌: %s 와 %s 가 같은 파일 %s 을 쓴다", prev, id, out)
		}
		seen[out] = id

		code, err := gen.Render(gen.Target{
			Package:  g.Pkg,
			GoName:   gen.GoName(name),
			APIID:    id,
			APIName:  api.Meta["API 명"],
			MenuPath: menu,
			Path:     api.Meta["URL"],
			Request:  reqTree,
			Response: resTree,
		})
		if err != nil {
			log.Fatalf("%s 렌더: %v", id, err)
		}
		if err := os.WriteFile(out, code, 0o644); err != nil {
			log.Fatalf("%s 쓰기: %v", out, err)
		}
		counts[g.Market+"/"+g.Pkg]++
	}

	total := 0
	for _, g := range gen.Groups {
		key := g.Market + "/" + g.Pkg
		fmt.Printf("%-24s %3d개\n", key, counts[key])
		total += counts[key]
	}
	fmt.Printf("%-24s %3d개\n", "합계", total)

	if len(missing) > 0 {
		// 이름표가 없으면 멈춘다. api-id 로 대충 이름을 지으면 나중에 바꿀 수 없다.
		log.Fatalf("이름표 없는 API %d개: %v", len(missing), missing)
	}
}
