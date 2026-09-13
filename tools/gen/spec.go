// Package gen 은 키움 공식 스펙에서 Go 클라이언트 코드를 만든다.
package gen

import (
	"encoding/json"
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
func BuildTree(fs []Field) []Node {
	// 섹션 헤더는 문서 표의 구분선이지 필드가 아니다.
	clean := make([]Field, 0, len(fs))
	for _, f := range fs {
		if f.IsSection || f.Element == "" {
			continue
		}
		clean = append(clean, f)
	}
	nodes, _ := build(clean, 0, 0)
	return nodes
}

// build 는 i 부터 depth 인 형제들을 모으고, 다음에 볼 위치를 돌려준다.
func build(fs []Field, i, depth int) ([]Node, int) {
	var out []Node
	for i < len(fs) {
		f := fs[i]
		if f.Depth < depth {
			return out, i
		}
		if f.Depth > depth {
			// 스펙이 한 단계를 건너뛴 경우. 버리지 않고 현재 깊이로 끌어올린다.
			f.Depth = depth
		}
		n := Node{Field: f, IsList: f.Type == "LIST"}
		i++
		if n.IsList {
			n.Children, i = build(fs, i, depth+1)
		}
		out = append(out, n)
	}
	return out, i
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
