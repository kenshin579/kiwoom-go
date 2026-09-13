# kiwoom-go 2단계 — 나머지 REST 227개 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 나머지 REST **227개**(국내 106 · 미국 121)를 생성해 동작시키고, **이중 주문 경로를 없앤다.**

**Architecture:** 401 재시도를 걷어내 주문이 두 번 들어갈 길을 막는다. 패키지가 25개로 늘어나므로 하위 클라이언트와 루트 배선까지 생성기가 찍어내, 사람이 손으로 관리할 것을 남기지 않는다.

**Tech Stack:** Go 1.25 · `text/template` · `go/format` · `httptest`

설계 문서: `docs/superpowers/specs/2026-09-14-kiwoom-go-phase2-design.md`
1단계 설계: `docs/superpowers/specs/2026-09-14-kiwoom-go-core-design.md`

---

## 파일 구조

| 파일 | 책임 | 상태 |
|---|---|---|
| `internal/transport/transport.go` | **401 재시도 제거** | 수정 |
| `internal/transport/transport_test.go` | 재시도 없음 확인 | 수정 |
| `tools/gen/groups.go` | **카테고리 25개 표** + 의도적 제외 목록 | 신규 |
| `tools/gen/groups_test.go` | 표의 완전성·결정성 | 신규 |
| `tools/gen/render_client.go` | 하위 `client.go` · 루트 `subclients.go` 렌더링 | 신규 |
| `tools/gen/render_client_test.go` | 골든 테스트 | 신규 |
| `tools/gen/main/main.go` | 25개 카테고리 순회, 세 종류 파일 출력 | 수정 |
| `client.go` | 필드 3개 제거 → `Clients` 임베딩 | 수정 |
| `subclients.go` | **생성물** — `Clients` + `newClients` | 신규(생성) |
| `domestic/*/client.go` · `overseas/*/client.go` | **생성물** | 신규(생성) |
| `domestic/*/{api}.go` · `overseas/*/{api}.go` | **생성물** 227개 | 신규(생성) |
| `README.md` | 지원 범위 갱신 | 수정 |

**최종 개수:** `domestic` 183(기존 77 + 신규 106) · `overseas` 121 · 합 **304**.

---

## Task 1: 401 재시도를 걷어낸다

**Files:**
- Modify: `internal/transport/transport.go` (`Do`)
- Test: `internal/transport/transport_test.go`

2단계에는 **돈이 움직이는 API 20개**(국내 주문 8 · 신용주문 4 · 미국 주문 5 · 환전 3)가 들어온다.
지금은 401 을 받으면 같은 요청을 한 번 더 보내는데, 첫 요청이 서버에 닿아 체결됐는데 응답만
401 로 보이면 **같은 주문이 두 번 들어간다.**

- [ ] **Step 1: 기존 테스트 두 개를 바꿔 쓴다**

`internal/transport/transport_test.go` 에서 `TestDo_401이면_한번만_재발급한다` 와
`TestDo_401이_계속되면_포기한다` **두 함수를 지우고** 아래 둘을 넣는다:

```go
func TestDo_401이면_토큰만_버리고_에러를_돌려준다(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"return_code":8005,"return_msg":"토큰 만료"}`))
	}))
	defer srv.Close()

	tok := &stubToken{val: "TK"}
	c := transport.New(srv.URL, srv.Client(), tok)
	_, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, nil)
	if err == nil {
		t.Fatal("에러여야 한다")
	}
	var ae *transport.APIError
	if !errors.As(err, &ae) || ae.StatusCode != http.StatusUnauthorized {
		t.Fatalf("401 APIError 여야 한다: %v", err)
	}
	// 재시도하면 주문이 두 번 들어갈 수 있다 — 정확히 한 번만 보내야 한다.
	if hits != 1 {
		t.Errorf("호출 = %d, want 1 (재시도 금지)", hits)
	}
	if atomic.LoadInt32(&tok.invalidated) != 1 {
		t.Errorf("Invalidate 호출 = %d, want 1", tok.invalidated)
	}
}

func TestDo_401_다음_호출은_새_토큰으로_나간다(t *testing.T) {
	// 401 로 토큰을 버렸으므로 다음 호출은 스스로 회복돼야 한다.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"return_code":8005,"return_msg":"토큰 만료"}`))
			return
		}
		_, _ = w.Write([]byte(`{"return_code":0,"return_msg":"정상"}`))
	}))
	defer srv.Close()

	tok := &stubToken{val: "TK"}
	c := transport.New(srv.URL, srv.Client(), tok)

	if _, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, nil); err == nil {
		t.Fatal("첫 호출은 에러여야 한다")
	}
	if _, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, nil); err != nil {
		t.Fatalf("둘째 호출은 성공해야 한다: %v", err)
	}
	if hits != 2 {
		t.Errorf("호출 = %d, want 2", hits)
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./internal/transport/ -run TestDo_401 -v`
Expected: FAIL — `호출 = 2, want 1` (아직 재시도가 살아 있다)

- [ ] **Step 3: 재시도를 걷어낸다**

`internal/transport/transport.go` 의 `Do` 를 교체:

```go
// Do 는 요청 한 건을 보내고 본문을 out 에 넣는다. out 이 nil 이면 본문을 버린다.
//
// **재시도하지 않는다.** 401 을 받으면 토큰만 버리고 에러를 돌려주므로, 다음 호출이 새
// 토큰을 받아 스스로 회복된다.
//
// 예전에는 401 에서 같은 요청을 한 번 더 보냈다. 읽기 전용일 때는 안전했지만, 주문 API 가
// 들어오면 첫 요청이 서버에 닿아 체결됐는데 응답만 401 로 보이는 경우 **같은 주문이 두 번**
// 들어간다. 어떤 API 가 돈을 움직이는지 스펙이 알려주지 않으므로 카테고리로 가르지 않고
// 규칙을 하나만 둔다. 토큰은 만료 60초 전에 미리 갱신하므로 401 자체가 드물다.
func (c *Client) Do(ctx context.Context, req Request, out any, opts ...Option) (Meta, error) {
	for _, fn := range opts {
		fn(&req)
	}
	meta, err := c.do(ctx, req, out)
	var ae *APIError
	if err != nil && errors.As(err, &ae) && ae.StatusCode == http.StatusUnauthorized {
		c.token.Invalidate()
	}
	return meta, err
}
```

- [ ] **Step 4: 통과를 확인한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
go build ./... && go vet ./... && gofmt -l . && go test ./... -v
go test ./internal/transport/ -race -count=3
```
Expected: 전부 PASS. **1단계 생성물 77개는 영향받지 않는다** — 시그니처가 그대로다.

- [ ] **Step 5: 커밋**

```bash
git add internal/transport/transport.go internal/transport/transport_test.go
git commit -m "fix: 401 재시도를 걷어내 이중 주문 경로를 없앤다" -- internal/transport/transport.go internal/transport/transport_test.go
```

---

## Task 2: 카테고리 → 패키지 표

**Files:**
- Create: `tools/gen/groups.go`, `tools/gen/groups_test.go`

API 이름은 공식 영문명을 쓰지만 **패키지 이름은 스펙이 주지 않는다.** `기관/외국인` 처럼
슬래시가 든 것도 있어 자동 변환은 불가능하다. 25줄 표로 명시한다.

**표에 없고 제외 목록에도 없는 카테고리를 만나면 멈춘다** — 3단계에서 새 카테고리가
들어올 때 조용히 빠지지 않게 하려는 것이다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`tools/gen/groups_test.go`:

```go
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
	if len(gen.Groups) != 25 {
		t.Errorf("카테고리 = %d개, want 25", len(gen.Groups))
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
```

- [ ] **Step 2: 실패를 확인한다**

Run: `cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools && go test ./gen/ -run TestGroups`
Expected: 컴파일 실패 — `undefined: gen.Groups`

- [ ] **Step 3: 구현한다**

`tools/gen/groups.go`:

```go
package gen

import "strings"

// Group 은 카테고리 하나의 생성 대상이다.
type Group struct {
	Menu   string // 스펙의 "메뉴 위치" 앞 두 단계. 예: "국내주식 > 시세"
	Market string // "domestic" | "overseas"
	Pkg    string // 패키지 이름. 예: "quote"
	Field  string // 루트 Clients 의 필드 이름. 예: "DomesticQuote"
	Korean string // doc 주석용. 예: "국내주식 시세"
}

// Groups 는 카테고리 → 패키지 표다.
//
// **여기만 지어낸 이름이다.** API 이름은 공식 영문명을 그대로 쓰지만(1단계 설계 §6.1)
// 패키지 이름은 스펙이 주지 않는다. `기관/외국인` 처럼 슬래시가 든 것도 있어 자동 변환은
// 불가능하다.
//
// 슬라이스로 두는 이유: 생성 순서가 맵 순회에 흔들리지 않아야 한다.
var Groups = []Group{
	{"국내주식 > 시세", "domestic", "quote", "DomesticQuote", "국내주식 시세"},
	{"국내주식 > 차트", "domestic", "chart", "DomesticChart", "국내주식 차트"},
	{"국내주식 > 종목정보", "domestic", "stock", "DomesticStock", "국내주식 종목정보"},
	{"국내주식 > 계좌", "domestic", "account", "DomesticAccount", "국내주식 계좌"},
	{"국내주식 > 순위정보", "domestic", "ranking", "DomesticRanking", "국내주식 순위정보"},
	{"국내주식 > ELW", "domestic", "elw", "DomesticELW", "국내주식 ELW"},
	{"국내주식 > ETF", "domestic", "etf", "DomesticETF", "국내주식 ETF"},
	{"국내주식 > 주문", "domestic", "order", "DomesticOrder", "국내주식 주문"},
	{"국내주식 > 업종", "domestic", "sector", "DomesticSector", "국내주식 업종"},
	{"국내주식 > 신용주문", "domestic", "creditorder", "DomesticCreditOrder", "국내주식 신용주문"},
	{"국내주식 > 대차거래", "domestic", "stocklending", "DomesticStockLending", "국내주식 대차거래"},
	{"국내주식 > 기관/외국인", "domestic", "investor", "DomesticInvestor", "국내주식 기관/외국인"},
	{"국내주식 > 테마", "domestic", "theme", "DomesticTheme", "국내주식 테마"},
	{"국내주식 > 관심종목", "domestic", "watchlist", "DomesticWatchlist", "국내주식 관심종목"},
	{"국내주식 > 공매도", "domestic", "shortsale", "DomesticShortSale", "국내주식 공매도"},

	{"미국주식 > 순위정보", "overseas", "ranking", "OverseasRanking", "미국주식 순위정보"},
	{"미국주식 > 종목정보", "overseas", "stock", "OverseasStock", "미국주식 종목정보"},
	{"미국주식 > 계좌", "overseas", "account", "OverseasAccount", "미국주식 계좌"},
	{"미국주식 > 차트", "overseas", "chart", "OverseasChart", "미국주식 차트"},
	{"미국주식 > 주문", "overseas", "order", "OverseasOrder", "미국주식 주문"},
	{"미국주식 > 시세", "overseas", "quote", "OverseasQuote", "미국주식 시세"},
	{"미국주식 > 환전", "overseas", "exchange", "OverseasExchange", "미국주식 환전"},
	{"미국주식 > 업종", "overseas", "sector", "OverseasSector", "미국주식 업종"},
	{"미국주식 > 관심종목", "overseas", "watchlist", "OverseasWatchlist", "미국주식 관심종목"},
	{"미국주식 > 투자정보", "overseas", "info", "OverseasInfo", "미국주식 투자정보"},
}

// skipped 는 **일부러** 생성하지 않는 카테고리다.
//
// 실시간·조건검색은 WebSocket 이라 프로토콜이 다르고(3단계), OAuth 는 internal/auth 가
// 손으로 다룬다. 여기 적어 두는 이유는 Groups 에 없는 카테고리를 전부 에러로 잡기 위해서다 —
// 그래야 새 카테고리가 조용히 빠지지 않는다.
var skipped = map[string]bool{
	"국내주식 > 실시간시세":     true,
	"미국주식 > 실시간시세":     true,
	"국내주식 > 조건검색":       true,
	"미국주식 > 조건검색":       true,
	"OAuth 인증 > 접근토큰발급": true,
	"OAuth 인증 > 접근토큰폐기": true,
}

// menuKey 는 "국내주식 > 시세 > 주식호가요청(ka10004)" 에서 앞 두 단계만 뗀다.
func menuKey(menu string) string {
	parts := strings.SplitN(menu, " > ", 3)
	if len(parts) < 2 {
		return menu
	}
	return parts[0] + " > " + parts[1]
}

// Lookup 은 메뉴 위치로 생성 대상을 찾는다.
func Lookup(menu string) (Group, bool) {
	key := menuKey(menu)
	for _, g := range Groups {
		if g.Menu == key {
			return g, true
		}
	}
	return Group{}, false
}

// Skipped 는 일부러 생성하지 않는 카테고리인지 알려준다.
func Skipped(menu string) bool { return skipped[menuKey(menu)] }
```

- [ ] **Step 4: 통과를 확인한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools
go test ./gen/ -run TestGroups -v
go build ./... && go vet ./... && gofmt -l .
```
Expected: 3개 PASS. 특히 `TestGroups_스펙의_모든_카테고리를_덮는다` 가 통과해야 한다 —
실패하면 표에 빠진 카테고리가 있다는 뜻이니 **멈추고 보고하라.**

- [ ] **Step 5: 커밋**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
git add tools/gen/groups.go tools/gen/groups_test.go
git commit -m "feat(gen): 카테고리 25개 표와 의도적 제외 목록" -- tools/gen/groups.go tools/gen/groups_test.go
```

---

## Task 3: 하위 클라이언트와 루트 배선을 생성물로

**Files:**
- Create: `tools/gen/render_client.go`, `tools/gen/render_client_test.go`

패키지가 25개가 되면 `client.go` 를 손으로 쓰고 루트에 배선하는 일이 25번 생긴다.
**깜빡하는 실수가 반드시 난다.** 생성기가 찍는다.

**import 별칭이 필수다** — `domestic/quote` 와 `overseas/quote` 가 둘 다 `quote` 라
별칭 없이는 컴파일되지 않는다. `<market><pkg>` 로 짓는다(`domesticquote`).

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`tools/gen/render_client_test.go`:

```go
package gen_test

import (
	"strings"
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func TestRenderSubClient(t *testing.T) {
	g := gen.Group{Menu: "국내주식 > 시세", Market: "domestic", Pkg: "quote",
		Field: "DomesticQuote", Korean: "국내주식 시세"}
	src, err := gen.RenderSubClient(g)
	if err != nil {
		t.Fatalf("RenderSubClient: %v", err)
	}
	got := string(src)
	for _, want := range []string{
		"// Code generated by tools/gen. DO NOT EDIT.",
		"// Package quote 는 키움 국내주식 시세 API 그룹이다.",
		"package quote",
		"type Client struct {",
		"http *transport.Client",
		"func New(hc *transport.Client) *Client",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 가 없다\n---\n%s", want, got)
		}
	}
}

func TestRenderClients(t *testing.T) {
	gs := []gen.Group{
		{Menu: "국내주식 > 시세", Market: "domestic", Pkg: "quote", Field: "DomesticQuote", Korean: "국내주식 시세"},
		{Menu: "미국주식 > 시세", Market: "overseas", Pkg: "quote", Field: "OverseasQuote", Korean: "미국주식 시세"},
	}
	src, err := gen.RenderClients(gs)
	if err != nil {
		t.Fatalf("RenderClients: %v", err)
	}
	got := string(src)
	for _, want := range []string{
		"// Code generated by tools/gen. DO NOT EDIT.",
		"package kiwoom",
		// 패키지명이 같으므로 별칭이 없으면 컴파일되지 않는다
		`domesticquote "github.com/kenshin579/kiwoom-go/domestic/quote"`,
		`overseasquote "github.com/kenshin579/kiwoom-go/overseas/quote"`,
		"type Clients struct {",
		"DomesticQuote *domesticquote.Client",
		"OverseasQuote *overseasquote.Client",
		"func newClients(tr *transport.Client) Clients {",
		"DomesticQuote: domesticquote.New(tr),",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q 가 없다\n---\n%s", want, got)
		}
	}
}

func TestRenderClients_빈목록도_컴파일된다(t *testing.T) {
	if _, err := gen.RenderClients(nil); err != nil {
		t.Fatalf("빈 목록: %v", err)
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools && go test ./gen/ -run TestRender.*Client`
Expected: 컴파일 실패 — `undefined: gen.RenderSubClient`

- [ ] **Step 3: 구현한다**

`tools/gen/render_client.go`:

```go
package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"text/template"
)

// Alias 는 루트 subclients.go 에서 쓸 import 별칭이다.
//
// domestic/quote 와 overseas/quote 가 둘 다 패키지 이름이 quote 라, 별칭 없이는
// 컴파일되지 않는다.
func (g Group) Alias() string { return g.Market + g.Pkg }

// ImportPath 는 이 그룹의 import 경로다.
func (g Group) ImportPath() string {
	return "github.com/kenshin579/kiwoom-go/" + g.Market + "/" + g.Pkg
}

// RenderSubClient 는 그룹 하나의 client.go 를 만든다.
func RenderSubClient(g Group) ([]byte, error) {
	var buf bytes.Buffer
	if err := subClientTmpl.Execute(&buf, g); err != nil {
		return nil, err
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt 실패(%s): %w\n%s", g.Pkg, err, buf.String())
	}
	return src, nil
}

// RenderClients 는 루트 subclients.go 를 만든다.
func RenderClients(gs []Group) ([]byte, error) {
	var buf bytes.Buffer
	if err := clientsTmpl.Execute(&buf, gs); err != nil {
		return nil, err
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt 실패(subclients): %w\n%s", err, buf.String())
	}
	return src, nil
}

var subClientTmpl = template.Must(template.New("subclient").Parse(`// Code generated by tools/gen. DO NOT EDIT.

// Package {{.Pkg}} 는 키움 {{.Korean}} API 그룹이다.
package {{.Pkg}}

import "github.com/kenshin579/kiwoom-go/internal/transport"

// Client 는 {{.Korean}} 하위 클라이언트다.
type Client struct {
	http *transport.Client
}

// New 는 kiwoom.NewClient 가 호출한다.
func New(hc *transport.Client) *Client { return &Client{http: hc} }
`))

var clientsTmpl = template.Must(template.New("clients").Parse(`// Code generated by tools/gen. DO NOT EDIT.

package kiwoom

import (
	"github.com/kenshin579/kiwoom-go/internal/transport"
{{range .}}	{{.Alias}} "{{.ImportPath}}"
{{end}})

// Clients 는 API 그룹별 하위 클라이언트다. Client 가 임베딩하므로
// c.DomesticQuote 처럼 바로 쓴다.
type Clients struct {
{{range .}}	// {{.Field}} 는 {{.Korean}}.
	{{.Field}} *{{.Alias}}.Client
{{end}}}

// newClients 는 NewClient 가 호출한다.
func newClients(tr *transport.Client) Clients {
	return Clients{
{{range .}}		{{.Field}}: {{.Alias}}.New(tr),
{{end}}	}
}
`))
```

- [ ] **Step 4: 통과를 확인한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools
go test ./gen/ -v
go build ./... && go vet ./... && gofmt -l .
```
Expected: 새 테스트 3개 + 기존 전부 PASS

- [ ] **Step 5: 커밋**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
git add tools/gen/render_client.go tools/gen/render_client_test.go
git commit -m "feat(gen): 하위 클라이언트와 루트 배선 렌더링" -- tools/gen/render_client.go tools/gen/render_client_test.go
```

---

## Task 4: 생성기를 25개 카테고리로 넓힌다

**Files:**
- Modify: `tools/gen/main/main.go`

- [ ] **Step 1: `main.go` 를 교체한다**

```go
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
```

- [ ] **Step 2: 컴파일만 확인한다 (아직 돌리지 마라)**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools
go build ./... && go vet ./... && gofmt -l .
```
Expected: 조용. **`go run ./gen/main` 은 아직 돌리지 마라** — 루트 `client.go` 가 아직
`Clients` 를 임베딩하지 않아 생성 후 컴파일이 깨진다(Task 5 에서 함께 한다).

- [ ] **Step 3: 커밋**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
git add tools/gen/main/main.go
git commit -m "feat(gen): 25개 카테고리 순회와 하위 클라이언트·배선 출력" -- tools/gen/main/main.go
```

---

## Task 5: 루트를 임베딩으로 바꾸고 227개를 생성한다

**Files:**
- Modify: `client.go`
- Create(생성): `subclients.go`, `domestic/*/client.go`, `overseas/*/client.go`, 생성물 227개

- [ ] **Step 1: 루트 `client.go` 를 고친다**

`Client` 구조체에서 하위 클라이언트 필드 3개를 **지우고** `Clients` 를 임베딩한다:

```go
// Client 는 키움 API 클라이언트다. 하위 클라이언트로 각 API 그룹에 접근한다.
//
// Clients 를 임베딩하므로 c.DomesticQuote 처럼 바로 쓴다. 그 목록은 생성물
// subclients.go 에 있다.
type Client struct {
	baseURL string
	http    *transport.Client

	Clients
}
```

import 에서 `domestic/chart`·`domestic/quote`·`domestic/stock` 세 줄을 **지운다**
(이제 `subclients.go` 가 import 한다).

`NewClient` 의 반환부를 교체:

```go
	tok := auth.New(o.baseURL, appKey, secretKey, hc)
	tr := transport.New(o.baseURL, hc, tok)
	return &Client{
		baseURL: o.baseURL,
		http:    tr,
		Clients: newClients(tr),
	}, nil
```

- [ ] **Step 2: 생성한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools
go run ./gen/main
```

기대 출력(카테고리별). **이 표와 다르면 멈추고 보고하라:**

```
domestic/quote            25개
domestic/chart            21개
domestic/stock            31개
domestic/account          33개
domestic/ranking          23개
domestic/elw              11개
domestic/etf               9개
domestic/order             8개
domestic/sector            6개
domestic/creditorder       4개
domestic/stocklending      4개
domestic/investor          3개
domestic/theme             2개
domestic/watchlist         2개
domestic/shortsale         1개
overseas/ranking          35개
overseas/stock            33개
overseas/account          28개
overseas/chart             7개
overseas/order             5개
overseas/quote             5개
overseas/exchange          3개
overseas/sector            2개
overseas/watchlist         2개
overseas/info              1개
합계                      304개
```

- [ ] **Step 3: 컴파일과 검사**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
gofmt -l .                 # 출력 없어야 한다
go build ./... && go vet ./... && go test ./...
```

**컴파일이 깨지면 생성기를 고쳐 다시 돌려라. 생성물을 손으로 고치지 마라.**
고쳤다면 무엇을 왜 고쳤는지 보고하라.

- [ ] **Step 4: 재생성 결정성을 확인한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools && go run ./gen/main >/dev/null
cd .. && git status --porcelain | grep -v '^ M client.go' | head
```
두 번째 실행이 아무것도 바꾸지 않아야 한다(첫 실행 결과와 동일).

- [ ] **Step 5: 커밋 — 세 번으로 나눈다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go

# (1) 배선
git add client.go subclients.go
git commit -m "feat: 하위 클라이언트를 Clients 임베딩으로" -- client.go subclients.go

# (2) 국내 106 + 기존 3개 패키지의 client.go 생성물화
git add domestic
git commit -m "feat: 국내주식 나머지 106개 생성" -- domestic

# (3) 미국 121
git add overseas
git commit -m "feat: 미국주식 121개 생성" -- overseas
```

---

## Task 6: 검증과 README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 외부 모듈에서 쓸 수 있는지 확인한다**

1단계에서 `internal` 누출로 연속조회가 막혔던 일이 있다. 새 패키지에서도 확인한다.

```bash
mkdir -p /tmp/kiwoom_p2 && cd /tmp/kiwoom_p2
cat > go.mod <<'EOF'
module example.com/p2

go 1.25

require github.com/kenshin579/kiwoom-go v0.0.0

replace github.com/kenshin579/kiwoom-go => /Users/frankoh/src/workspace_moneyflow/kiwoom-go
EOF
cat > main.go <<'EOF'
package main

import (
	"context"
	"fmt"

	"github.com/kenshin579/kiwoom-go"
	"github.com/kenshin579/kiwoom-go/overseas/ranking"
)

func main() {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithMock())
	if err != nil {
		panic(err)
	}
	// 1단계 경로가 임베딩 뒤에도 그대로 도는지
	_ = c.DomesticQuote
	// 2단계 새 패키지
	var _ *ranking.Client = c.OverseasRanking
	// 연속조회 옵션이 외부에서 만들어지는지
	var meta kiwoom.Meta
	_ = kiwoom.WithCont(meta)
	_ = context.Background()
	fmt.Println("ok")
}
EOF
go mod tidy && go build ./... && echo "외부 모듈 빌드 성공"
```

**실패하면 에러를 그대로 보고하라.** 끝나면 `rm -rf /tmp/kiwoom_p2`.

- [ ] **Step 2: README 의 지원 범위를 갱신한다**

`README.md` 에서 지원 범위를 적은 부분을 찾아 다음으로 바꾼다(주변 문장은 그대로 두고
숫자와 목록만 고친다):

- 생성된 REST **304개** — 국내주식 183 · 미국주식 121
- 아직 아닌 것: 실시간 WebSocket 23 · 조건검색 8

그리고 **재시도 정책 문단을 고친다.** 지금 README 는 "401 만 1회 재발급" 이라고 적혀 있는데
더 이상 사실이 아니다:

```markdown
## 재시도

**재시도하지 않는다.** 401 을 받으면 토큰만 버리고 에러를 돌려주므로, 다음 호출이 새 토큰을
받아 스스로 회복된다.

같은 요청을 다시 보내지 않는 이유는 주문 때문이다 — 첫 요청이 서버에 닿아 체결됐는데 응답만
401 로 보이면 **같은 주문이 두 번** 들어간다. 어떤 API 가 돈을 움직이는지 스펙이 알려주지
않으므로 규칙을 하나만 둔다. 토큰은 만료 60초 전에 미리 갱신하므로 401 자체가 드물다.

429·5xx 도 재시도하지 않는다. 키움의 정책을 확인하기 전에 추측으로 재시도하면 한도를 더
빨리 태운다 — 속도 조절은 호출자 책임이다.
```

새 패키지 목록도 한 줄 덧붙인다(전부 나열하지 말고 시장별 개수와 대표 몇 개만).

- [ ] **Step 3: 전체 검증**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
go build ./... && go vet ./... && go test ./... && gofmt -l .
cd tools && go build ./... && go vet ./... && go test ./... && gofmt -l . && cd ..
go list ./... | grep tools && echo "❌ tools 포함됨" || echo "✅ tools 제외됨"
git status --porcelain
```

- [ ] **Step 4: 커밋**

```bash
git add README.md
git commit -m "docs: README — 2단계 범위와 재시도 정책 갱신" -- README.md
```

**푸시하지 마라. PR 도 만들지 마라** — 컨트롤러가 한다.

---

## 자체 검토 메모

- 설계 §4(재시도 제거) → Task 1. §5(하위 클라이언트 생성물화) → Task 3·5.
  §6(이름표) → Task 2. §3(범위·개수) → Task 5 Step 2 의 기대 표. §7(커밋 분리) → Task 5 Step 5.
  §8(검증) → Task 5 Step 3·4, Task 6. §9(위험) → Task 2 의 완전성 테스트, Task 5 의 개수·결정성,
  Task 6 의 외부 모듈 확인.
- **1단계 `domestic/{quote,chart,stock}/client.go` 가 생성물로 덮인다.** 손으로 쓴 것과 내용이
  거의 같지만 `DO NOT EDIT` 헤더가 붙는다. Task 5 Step 5 의 `git add domestic` 이 그 변경을
  함께 담는다.
- **Task 4 에서 생성기를 돌리지 않는 이유**를 Step 2 에 적었다 — 루트가 아직 임베딩 전이라
  생성 직후 컴파일이 깨진다. Task 5 에서 순서대로 한다.
- `go test ./...` 는 `domestic/quote/generated_test.go` 를 그대로 돌린다. 1단계 테스트가
  2단계 변경에도 통과해야 한다(회귀 확인).
