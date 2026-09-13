// Package gen 은 키움 공식 스펙에서 Go 클라이언트 코드를 만든다.
package gen

import (
	"encoding/json"
	"fmt"
	"os"
)

// Field 는 스펙의 필드 한 줄. JSON 키가 한글이라 태그로 맞춘다.
type Field struct {
	Element     string `json:"element"`
	Depth       int    `json:"depth"`
	IsSection   bool   `json:"is_section"`
	Korean      string `json:"한글명"`
	Type        string `json:"type"`
	Required    string `json:"required"`
	Length      string `json:"length"`
	Description string `json:"description"`
}

// Node 는 트리로 접은 필드. LIST 필드가 자식을 갖는다.
type Node struct {
	Field
	IsList   bool
	Children []Node
}

// BuildTree 는 평평한 필드 배열을 depth 기준 트리로 접는다.
//
// 스펙은 중첩을 depth 숫자로만 나타낸다 — LIST 필드 뒤에 오는 **더 깊은** 필드들이 그
// 원소의 멤버다. 같거나 얕은 깊이가 나오면 그 목록은 끝난 것이다.
// 응답은 최대 3단계(0·1·2)이고 depth 2 필드가 611개 있어 재귀가 필요하다.
//
// 다만 그 611개는 **스펙 파일**의 이야기다. 지금 생성하는 304개(REST)에는 depth >= 2 필드가
// 하나도 없다 — depth 2 는 전부 제외한 실시간 WebSocket 카테고리에 산다. 즉 이 재귀는
// 현재 생성물에서 **한 번도 쓰이지 않고**, 3단계(실시간)가 첫 실사용이 된다.
// 그때까지 이 코드를 붙잡아 주는 그물은 tools/gen 의 단위 테스트뿐이다(생성물 비교로는
// 안 잡힌다). 재귀를 손볼 때는 spec_test.go 를 먼저 늘려라.
//
// 컨테이너로 인정하는 것은 `LIST` 뿐이다. 그 밖의 타입(`Object`·`List<Map>`)이 자식을
// 달고 있으면 **에러**를 돌려준다 — 예전에는 그 자식들을 현재 깊이로 끌어올렸는데,
// 그러면 최상위 형제로 조용히 옮겨붙어 구조가 틀린 채 생성물이 나왔다.
// 스펙 전체에 "진짜 깊이 건너뛰기" 는 0건이므로, 이 에러는 정당한 입력을 막지 않는다.
func BuildTree(fs []Field) ([]Node, error) {
	// 섹션 헤더는 문서 표의 구분선이지 필드가 아니다.
	clean := make([]Field, 0, len(fs))
	for _, f := range fs {
		if f.IsSection || f.Element == "" {
			continue
		}
		clean = append(clean, f)
	}
	nodes, _, err := build(clean, 0, 0)
	return nodes, err
}

// build 는 i 부터 depth 인 형제들을 모으고, 다음에 볼 위치를 돌려준다.
func build(fs []Field, i, depth int) ([]Node, int, error) {
	var out []Node
	for i < len(fs) {
		f := fs[i]
		if f.Depth < depth {
			return out, i, nil
		}
		if f.Depth > depth {
			// 여기 오는 것은 LIST 가 아닌 필드가 자식을 달고 있다는 뜻이다.
			// 끌어올려 붙이면 구조가 조용히 틀어지므로 멈춘다.
			return nil, i, fmt.Errorf(
				"gen: %q(depth %d)가 depth %d 자리에 나왔다 — 앞선 필드가 LIST 가 아닌데 자식을 갖는다(Object·List<Map> 등). 이 타입을 다루도록 build 를 넓히거나 해당 API 를 범위에서 빼라",
				f.Element, f.Depth, depth)
		}
		n := Node{Field: f, IsList: f.Type == "LIST"}
		i++
		if n.IsList {
			var err error
			n.Children, i, err = build(fs, i, depth+1)
			if err != nil {
				return nil, i, err
			}
		}
		out = append(out, n)
	}
	return out, i, nil
}

// API 는 스펙의 API 한 건.
type API struct {
	Meta     map[string]string `json:"meta"`
	Request  Body              `json:"request"`
	Response Body              `json:"response"`
}

// Body 는 요청/응답의 헤더·본문 필드.
type Body struct {
	Header []Field `json:"header"`
	Body   []Field `json:"body"`
}

// Spec 은 벤더링한 스펙 전체.
type Spec struct {
	APIs map[string]API `json:"apis"`
}

// LoadSpec 은 스펙 JSON 을 읽는다.
func LoadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// LoadNames 는 api-id → 공식 영문명 표를 읽는다.
func LoadNames(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
