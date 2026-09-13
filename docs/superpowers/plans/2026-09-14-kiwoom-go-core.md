# kiwoom-go 코어 + 생성기 + 국내주식 77개 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 키움 REST API 337개를 전부 다룰 수 있는 뼈대(인증·전송·변환)와 **스펙 기반 코드 생성기**를 세우고, 국내주식 시세·차트·종목정보 **77개**를 생성해 동작시킨다.

**Architecture:** 337개가 전부 POST·URL 29개·`api-id` 헤더 구분이라 전송이 함수 하나로 수렴한다. 손으로 쓰는 것은 코어뿐이고, 요청·응답 구조체와 메서드는 벤더링한 공식 스펙에서 생성한다. 생성기와 3.7MB 스펙은 `tools/` 별도 모듈에 둬 라이브러리 사용자가 내려받지 않게 한다.

**Tech Stack:** Go 1.25 · `shopspring/decimal` · 표준 라이브러리(`text/template`, `go/format`, `httptest`)

설계 문서: `docs/superpowers/specs/2026-09-14-kiwoom-go-core-design.md`

---

## 파일 구조

| 파일 | 책임 | 모듈 |
|---|---|---|
| `go.mod`, `LICENSE`, `.gitignore` | 모듈·라이선스 | 루트 |
| `internal/transport/transport.go` | POST · `api-id` · 연속조회 · 에러 매핑 · 401 1회 재발급 | 루트 |
| `internal/auth/auth.go` | 토큰 발급·만료 60초 전 갱신 | 루트 |
| `config.go` | `Option`, 환경변수 로딩, 운영/모의 도메인 | 루트 |
| `errors.go` | `APIError` 별칭, `IsCode`, 오류코드 상수 | 루트 |
| `client.go` | 루트 `Client` — 하위 클라이언트 노출 | 루트 |
| `convert.go` | `Decimal`/`Int`/`Date`/`DateTime` + `OK` 짝 | 루트 |
| `domestic/{quote,chart,stock}/*.go` | **생성물** — API 당 파일 하나 | 루트 |
| `tools/go.mod` | 생성기 모듈(라이브러리에서 제외) | tools |
| `tools/spec/kiwoom_api_spec.json` | 공식 스펙 사본 | tools |
| `tools/spec/api_names.json` | `api-id → 공식 영문명` | tools |
| `tools/gen/{parse,tree,render,main}.go` | 생성기 | tools |

**의존 방향:** 생성물(`domestic/...`) → `internal/transport`. 루트 `client.go` → 생성물.
생성물은 루트를 import 하지 않는다(순환 방지). `convert.go` 는 **사용자용**이지 생성물용이 아니다.

---

## Task 1: 모듈 초기화

**Files:** Create `go.mod`, `LICENSE`, `.gitignore`

- [ ] **Step 1: 모듈과 라이선스**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
go mod init github.com/kenshin579/kiwoom-go
go get github.com/shopspring/decimal@latest
```

`.gitignore`:

```
/kiwoom-go
*.test
.DS_Store
```

`LICENSE` — MIT, 저작권자 `kenshin579`, 연도 2026. 형제 저장소(`ecos-go/LICENSE`)의 본문을
그대로 쓰되 연도·이름만 맞춘다.

- [ ] **Step 2: 확인**

Run: `go build ./... && go vet ./...`
Expected: 오류 없음(아직 코드가 없으므로 조용히 통과)

- [ ] **Step 3: 커밋**

```bash
git add go.mod go.sum LICENSE .gitignore
git commit -m "chore: Go 모듈 초기화와 MIT 라이선스" -- go.mod go.sum LICENSE .gitignore
```

---

## Task 2: 문자열 변환 헬퍼

**Files:** Create `convert.go`, `convert_test.go`

**이 라이브러리에서 틀리면 조용한 유일한 자리다.** 응답 필드가 전부 문자열이라, 사용자가
값을 숫자로 바꾸는 길은 여기 하나뿐이다.

규칙:
- 앞뒤 공백과 천 단위 콤마를 제거한다.
- 앞의 `+` 를 떼어낸다(키움은 `"+1234"` 를 보낸다).
- **빈 문자열은 제로값이고 `ok=false`** 다 — "값이 없음" 과 "0" 을 구분할 수 있어야 한다.
- 파싱 실패는 제로값 + `ok=false`. **panic 하지 않는다.**
- 날짜는 KST 로 해석한다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`convert_test.go`:

```go
package kiwoom_test

import (
	"testing"
	"time"

	"github.com/kenshin579/kiwoom-go"
)

func TestDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1234", "1234", true},
		{"+1234", "1234", true},   // 키움은 부호를 붙여 보낸다
		{"-56", "-56", true},
		{"0012", "12", true},      // 앞자리 0
		{"1,234,567", "1234567", true},
		{"  789  ", "789", true},
		{"12.34", "12.34", true},
		{"", "0", false},          // 값 없음 — 0 과 구분된다
		{"abc", "0", false},
		{"--1", "0", false},
	}
	for _, c := range cases {
		got, ok := kiwoom.DecimalOK(c.in)
		if ok != c.ok {
			t.Errorf("DecimalOK(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
		if got.String() != c.want {
			t.Errorf("DecimalOK(%q) = %s, want %s", c.in, got.String(), c.want)
		}
		if kiwoom.Decimal(c.in).String() != c.want {
			t.Errorf("Decimal(%q) = %s, want %s", c.in, kiwoom.Decimal(c.in).String(), c.want)
		}
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1234", 1234, true},
		{"+1234", 1234, true},
		{"-56", -56, true},
		{"0012", 12, true},
		{"1,234", 1234, true},
		{"", 0, false},
		{"12.34", 0, false}, // 정수가 아니다
	}
	for _, c := range cases {
		got, ok := kiwoom.IntOK(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("IntOK(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDate(t *testing.T) {
	kst := time.FixedZone("KST", 9*60*60)
	got, ok := kiwoom.DateOK("20260913")
	if !ok {
		t.Fatal("DateOK(20260913) ok=false")
	}
	want := time.Date(2026, 9, 13, 0, 0, 0, 0, kst)
	if !got.Equal(want) {
		t.Errorf("DateOK = %v, want %v", got, want)
	}
	for _, bad := range []string{"", "2026091", "20261301", "abcdefgh"} {
		if _, ok := kiwoom.DateOK(bad); ok {
			t.Errorf("DateOK(%q) ok=true, want false", bad)
		}
	}
}

func TestDateTime(t *testing.T) {
	kst := time.FixedZone("KST", 9*60*60)
	got, ok := kiwoom.DateTimeOK("20241107083713")
	if !ok {
		t.Fatal("DateTimeOK ok=false")
	}
	want := time.Date(2024, 11, 7, 8, 37, 13, 0, kst)
	if !got.Equal(want) {
		t.Errorf("DateTimeOK = %v, want %v", got, want)
	}
	if _, ok := kiwoom.DateTimeOK("20241107"); ok {
		t.Error("길이가 다르면 false 여야 한다")
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./... -run 'TestDecimal|TestInt|TestDate'`
Expected: 컴파일 실패 — `undefined: kiwoom.DecimalOK`

- [ ] **Step 3: 구현한다**

`convert.go`:

```go
// Package kiwoom 은 키움증권 REST API 의 Go 클라이언트다.
package kiwoom

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// KST 는 키움이 쓰는 시간대. 날짜·시각 문자열은 전부 이 기준으로 해석한다.
var KST = time.FixedZone("KST", 9*60*60)

// clean 은 키움 숫자 문자열의 껍데기를 벗긴다.
//
// 키움은 부호를 붙여(`"+1234"`) 보내고, 자리를 채우려 앞에 0 을 두며(`"0012"`),
// 표시용 콤마가 섞여 오는 필드도 있다. 이 셋을 한 곳에서 처리해 사용자가 각자 밟지 않게 한다.
func clean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	return strings.TrimPrefix(s, "+")
}

// DecimalOK 는 금액·수량 문자열을 decimal 로 바꾼다.
// 빈 문자열은 (0, false) — "값 없음" 과 "0" 을 구분하기 위해서다.
func DecimalOK(s string) (decimal.Decimal, bool) {
	c := clean(s)
	if c == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(c)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

// Decimal 은 DecimalOK 의 값만 쓰는 축약형. 실패하면 0 이다.
func Decimal(s string) decimal.Decimal { d, _ := DecimalOK(s); return d }

// IntOK 는 정수 문자열을 int64 로 바꾼다. 소수점이 있으면 실패다.
func IntOK(s string) (int64, bool) {
	c := clean(s)
	if c == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Int 는 IntOK 의 값만 쓰는 축약형. 실패하면 0 이다.
func Int(s string) int64 { n, _ := IntOK(s); return n }

// DateOK 는 YYYYMMDD 를 KST 자정으로 바꾼다.
func DateOK(s string) (time.Time, bool) { return parseTime(s, "20060102") }

// Date 는 DateOK 의 값만 쓰는 축약형. 실패하면 제로 time.Time 이다.
func Date(s string) time.Time { t, _ := DateOK(s); return t }

// DateTimeOK 는 YYYYMMDDHHMMSS 를 KST 시각으로 바꾼다.
func DateTimeOK(s string) (time.Time, bool) { return parseTime(s, "20060102150405") }

// DateTime 은 DateTimeOK 의 값만 쓰는 축약형. 실패하면 제로 time.Time 이다.
func DateTime(s string) time.Time { t, _ := DateTimeOK(s); return t }

// parseTime 은 길이를 먼저 확인한다 — time.Parse 는 "20261301" 같은 값을 월 13 으로
// 받아들이지 않지만, 길이가 짧은 값을 부분 일치로 통과시키는 함정이 있다.
func parseTime(s, layout string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) != len(layout) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(layout, s, KST)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

- [ ] **Step 4: 통과를 확인한다**

Run: `go test ./... -v -run 'TestDecimal|TestInt|TestDate'`
Expected: 4개 PASS

- [ ] **Step 5: 커밋**

```bash
git add convert.go convert_test.go go.mod go.sum
git commit -m "feat: 응답 문자열 변환 헬퍼" -- convert.go convert_test.go go.mod go.sum
```

---

## Task 3: 오류 타입

**Files:** Create `errors.go`, `errors_test.go`

에러는 두 층을 하나로 모은다 — HTTP 전송 실패와, 200 으로 오지만 본문 `return_code != 0` 인 경우다.
**키움은 업무 오류를 HTTP 200 + `return_code` 로 돌려주므로**, 상태코드만 보면 실패를 놓친다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`errors_test.go`:

```go
package kiwoom_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

func TestAPIError(t *testing.T) {
	err := &kiwoom.APIError{StatusCode: 200, ReturnCode: 1505, ReturnMsg: "해당 API ID는 존재하지 않습니다"}

	if !kiwoom.IsCode(err, 1505) {
		t.Error("IsCode(1505) = false")
	}
	if kiwoom.IsCode(err, 1501) {
		t.Error("IsCode(1501) = true, want false")
	}
	if kiwoom.IsCode(errors.New("boom"), 1505) {
		t.Error("APIError 가 아니면 false 여야 한다")
	}

	wrapped := fmt.Errorf("조회 실패: %w", err)
	if !kiwoom.IsCode(wrapped, 1505) {
		t.Error("감싼 에러에서도 찾아야 한다")
	}
	if err.Error() == "" {
		t.Error("Error() 가 비어 있다")
	}
}

func TestAPIError_HTTPOnly(t *testing.T) {
	// 본문을 못 읽은 전송 실패 — ReturnCode 가 0 이어도 에러다
	err := &kiwoom.APIError{StatusCode: 500, ReturnMsg: "Internal Server Error"}
	if kiwoom.IsCode(err, 0) {
		t.Error("ReturnCode 0 을 코드로 취급하면 안 된다")
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./... -run TestAPIError`
Expected: 컴파일 실패 — `undefined: kiwoom.APIError`

- [ ] **Step 3: 구현한다**

`errors.go`:

```go
package kiwoom

import (
	"errors"
	"fmt"
)

// APIError 는 키움 API 호출 실패다.
//
// 두 층을 하나로 모은다. HTTP 자체가 실패한 경우(StatusCode != 200)와, HTTP 200 으로 왔지만
// 본문의 return_code 가 0 이 아닌 경우다. **키움은 업무 오류를 200 + return_code 로 돌려주므로**
// 상태코드만 보면 실패를 놓친다.
type APIError struct {
	StatusCode int    // HTTP 상태코드
	ReturnCode int    // 본문 return_code. 전송 실패라 본문을 못 읽었으면 0
	ReturnMsg  string // 본문 return_msg 또는 HTTP 상태 문구
	APIID      string // 호출한 api-id(어느 API 가 실패했는지)
	Body       string // 파싱 실패 시 원문 일부(최대 512바이트)
}

func (e *APIError) Error() string {
	if e.ReturnCode != 0 {
		return fmt.Sprintf("kiwoom: %s 실패 (return_code=%d): %s", e.APIID, e.ReturnCode, e.ReturnMsg)
	}
	return fmt.Sprintf("kiwoom: %s 실패 (HTTP %d): %s", e.APIID, e.StatusCode, e.ReturnMsg)
}

// IsCode 는 err 가 주어진 키움 return_code 의 *APIError 인지 판별한다.
// code 0 은 "오류 없음" 이므로 항상 false 다.
func IsCode(err error, code int) bool {
	if code == 0 {
		return false
	}
	var ae *APIError
	return errors.As(err, &ae) && ae.ReturnCode == code
}
```

- [ ] **Step 4: 통과를 확인한다**

Run: `go test ./... -v -run TestAPIError`
Expected: 2개 PASS

- [ ] **Step 5: 커밋**

```bash
git add errors.go errors_test.go
git commit -m "feat: APIError 와 IsCode" -- errors.go errors_test.go
```

---

## Task 4: 토큰 발급·갱신

**Files:** Create `internal/auth/auth.go`, `internal/auth/auth_test.go`

`POST /oauth2/token` 에 `{"grant_type":"client_credentials","appkey":...,"secretkey":...}` 를 보내면
`{"token":"...","token_type":"bearer","expires_dt":"20241107083713","return_code":0}` 가 온다.

**필드 이름이 `access_token` 이 아니라 `token` 이고, 만료가 초가 아니라 시각 문자열**이다.
OAuth2 표준과 다르므로 표준 라이브러리를 쓰지 않는다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`internal/auth/auth_test.go`:

```go
package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kenshin579/kiwoom-go/internal/auth"
)

func server(t *testing.T, expires string, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("경로 = %s", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "client_credentials" || body["appkey"] != "AK" || body["secretkey"] != "SK" {
			t.Errorf("본문 = %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "T1", "token_type": "bearer", "expires_dt": expires, "return_code": 0,
		})
	}))
}

func TestToken_캐시한다(t *testing.T) {
	var calls int32
	future := time.Now().In(time.FixedZone("KST", 9*3600)).Add(time.Hour).Format("20060102150405")
	srv := server(t, future, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	for i := 0; i < 3; i++ {
		tok, err := s.Token(context.Background())
		if err != nil || tok != "T1" {
			t.Fatalf("Token = %q, %v", tok, err)
		}
	}
	if calls != 1 {
		t.Errorf("발급 호출 = %d, want 1 (캐시돼야 한다)", calls)
	}
}

func TestToken_만료전에_갱신한다(t *testing.T) {
	var calls int32
	// 30초 뒤 만료 — 60초 여유보다 짧으므로 매번 새로 받아야 한다
	soon := time.Now().In(time.FixedZone("KST", 9*3600)).Add(30 * time.Second).Format("20060102150405")
	srv := server(t, soon, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	_, _ = s.Token(context.Background())
	_, _ = s.Token(context.Background())
	if calls != 2 {
		t.Errorf("발급 호출 = %d, want 2 (만료 임박이면 갱신)", calls)
	}
}

func TestToken_Invalidate(t *testing.T) {
	var calls int32
	future := time.Now().In(time.FixedZone("KST", 9*3600)).Add(time.Hour).Format("20060102150405")
	srv := server(t, future, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	_, _ = s.Token(context.Background())
	s.Invalidate()
	_, _ = s.Token(context.Background())
	if calls != 2 {
		t.Errorf("발급 호출 = %d, want 2 (Invalidate 후 재발급)", calls)
	}
}

func TestToken_발급실패(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"return_code":3,"return_msg":"appkey 오류"}`))
	}))
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	if _, err := s.Token(context.Background()); err == nil {
		t.Fatal("에러여야 한다")
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./internal/auth/ -v`
Expected: 컴파일 실패 — `undefined: auth.New`

- [ ] **Step 3: 구현한다**

`internal/auth/auth.go`:

```go
// Package auth 는 키움 OAuth2 접근토큰을 발급·캐시한다.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// refreshMargin 은 만료 이 시간 전에 미리 갱신한다.
// 호출 도중 만료돼 401 을 받는 것보다, 조금 일찍 받는 편이 싸다.
const refreshMargin = 60 * time.Second

var kst = time.FixedZone("KST", 9*60*60)

// Source 는 토큰을 캐시하는 발급기. 동시 호출에 안전하다.
type Source struct {
	baseURL   string
	appKey    string
	secretKey string
	hc        *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// New 는 토큰 발급기를 만든다.
func New(baseURL, appKey, secretKey string, hc *http.Client) *Source {
	return &Source{baseURL: baseURL, appKey: appKey, secretKey: secretKey, hc: hc}
}

// Invalidate 는 캐시한 토큰을 버린다. 401 을 받았을 때 호출한다.
func (s *Source) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token, s.expiresAt = "", time.Time{}
}

// Token 은 유효한 토큰을 돌려준다. 없거나 만료가 임박했으면 새로 받는다.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Now().Before(s.expiresAt.Add(-refreshMargin)) {
		return s.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"grant_type": "client_credentials",
		"appkey":     s.appKey,
		"secretkey":  s.secretKey,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/oauth2/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	resp, err := s.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("kiwoom: 토큰 발급 요청 실패: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Token      string `json:"token"`
		ExpiresDt  string `json:"expires_dt"`
		ReturnCode int    `json:"return_code"`
		ReturnMsg  string `json:"return_msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("kiwoom: 토큰 응답 파싱 실패(HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || out.ReturnCode != 0 || out.Token == "" {
		return "", fmt.Errorf("kiwoom: 토큰 발급 실패(HTTP %d, return_code=%d): %s",
			resp.StatusCode, out.ReturnCode, out.ReturnMsg)
	}

	exp, err := time.ParseInLocation("20060102150405", out.ExpiresDt, kst)
	if err != nil {
		// 만료 시각을 못 읽으면 캐시하지 않는다 — 만료된 토큰을 계속 쓰는 것보다 낫다.
		return out.Token, nil
	}
	s.token, s.expiresAt = out.Token, exp
	return s.token, nil
}
```

- [ ] **Step 4: 통과를 확인한다**

Run: `go test ./internal/auth/ -v`
Expected: 4개 PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/auth
git commit -m "feat: OAuth2 접근토큰 발급과 캐시" -- internal/auth
```

---

## Task 5: 전송 계층

**Files:** Create `internal/transport/transport.go`, `internal/transport/transport_test.go`

**337개를 이 함수 하나가 실어 나른다.** 전부 POST 이고 URL 은 29개뿐이며 구분은 `api-id` 헤더다.
연속조회도 전부 같다 — 응답 헤더 `cont-yn`·`next-key` 를 다음 요청 헤더에 그대로 넣는다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`internal/transport/transport_test.go`:

```go
package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kenshin579/kiwoom-go/internal/transport"
)

type stubToken struct {
	val        string
	invalidated int32
}

func (s *stubToken) Token(context.Context) (string, error) { return s.val, nil }
func (s *stubToken) Invalidate()                           { atomic.AddInt32(&s.invalidated, 1) }

func TestDo_헤더와_본문(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("메서드 = %s", r.Method)
		}
		if got := r.Header.Get("api-id"); got != "ka10004" {
			t.Errorf("api-id = %q", got)
		}
		if got := r.Header.Get("authorization"); got != "Bearer TK" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("cont-yn"); got != "Y" {
			t.Errorf("cont-yn = %q", got)
		}
		if got := r.Header.Get("next-key"); got != "K1" {
			t.Errorf("next-key = %q", got)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stk_cd"] != "005930" {
			t.Errorf("본문 = %v", body)
		}
		w.Header().Set("cont-yn", "Y")
		w.Header().Set("next-key", "K2")
		_, _ = w.Write([]byte(`{"return_code":0,"return_msg":"정상","bid_req_base_tm":"161000"}`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	var out struct {
		BidReqBaseTm string `json:"bid_req_base_tm"`
	}
	meta, err := c.Do(context.Background(), transport.Request{
		APIID: "ka10004", Path: "/api/dostk/mrkcond",
		Body:    map[string]string{"stk_cd": "005930"},
		ContYN:  "Y", NextKey: "K1",
	}, &out)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.BidReqBaseTm != "161000" {
		t.Errorf("본문 파싱 = %q", out.BidReqBaseTm)
	}
	if meta.ContYN != "Y" || meta.NextKey != "K2" {
		t.Errorf("연속조회 = %+v", meta)
	}
}

func TestDo_본문오류는_200이어도_에러다(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"return_code":1505,"return_msg":"해당 API ID는 존재하지 않습니다"}`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	var out struct{}
	_, err := c.Do(context.Background(), transport.Request{APIID: "ka99999", Path: "/x"}, &out)
	if err == nil {
		t.Fatal("에러여야 한다 — 키움은 업무 오류를 HTTP 200 으로 보낸다")
	}
	var ae *transport.APIError
	if !asAPIError(err, &ae) || ae.ReturnCode != 1505 {
		t.Fatalf("APIError(1505) 여야 한다: %v", err)
	}
}

func TestDo_401이면_한번만_재발급한다(t *testing.T) {
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
	var out struct{}
	if _, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, &out); err != nil {
		t.Fatalf("재시도로 성공해야 한다: %v", err)
	}
	if hits != 2 {
		t.Errorf("호출 = %d, want 2", hits)
	}
	if atomic.LoadInt32(&tok.invalidated) != 1 {
		t.Errorf("Invalidate 호출 = %d, want 1", tok.invalidated)
	}
}

func TestDo_401이_계속되면_포기한다(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"return_code":8005,"return_msg":"토큰 만료"}`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	var out struct{}
	if _, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, &out); err == nil {
		t.Fatal("두 번째도 401 이면 에러여야 한다")
	}
	if hits != 2 {
		t.Errorf("호출 = %d, want 2 (무한 재시도 금지)", hits)
	}
}

func asAPIError(err error, target **transport.APIError) bool {
	ae, ok := err.(*transport.APIError)
	if ok {
		*target = ae
	}
	return ok
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./internal/transport/ -v`
Expected: 컴파일 실패 — `undefined: transport.New`

- [ ] **Step 3: 구현한다**

`internal/transport/transport.go`:

```go
// Package transport 는 키움 REST 호출 하나를 담당한다.
//
// 키움 API 337개는 **전부 POST** 이고 URL 이 29개뿐이며 구분은 `api-id` 헤더가 한다.
// 그래서 이 함수 하나가 전부를 실어 나르고, 생성되는 코드는 얇은 껍데기가 된다.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// bodyPeek 은 파싱 실패 시 에러에 담을 원문 길이.
const bodyPeek = 512

// TokenSource 는 접근토큰을 준다. internal/auth.Source 가 만족한다.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
}

// APIError 는 호출 실패다. 루트 패키지가 별칭으로 다시 내보낸다.
type APIError struct {
	StatusCode int
	ReturnCode int
	ReturnMsg  string
	APIID      string
	Body       string
}

func (e *APIError) Error() string {
	if e.ReturnCode != 0 {
		return fmt.Sprintf("kiwoom: %s 실패 (return_code=%d): %s", e.APIID, e.ReturnCode, e.ReturnMsg)
	}
	return fmt.Sprintf("kiwoom: %s 실패 (HTTP %d): %s", e.APIID, e.StatusCode, e.ReturnMsg)
}

// Request 는 호출 한 건.
type Request struct {
	APIID   string // api-id 헤더 (예: ka10004)
	Path    string // URL 경로 (예: /api/dostk/mrkcond)
	Body    any    // 요청 본문(JSON)
	ContYN  string // 연속조회여부. 첫 호출은 빈 값
	NextKey string // 연속조회키. 첫 호출은 빈 값
}

// Meta 는 응답 헤더에서 읽은 연속조회 정보.
// ContYN 이 "Y" 면 NextKey 를 다음 요청에 넣어 이어서 조회한다.
type Meta struct {
	ContYN  string
	NextKey string
}

// Client 는 전송기.
type Client struct {
	baseURL string
	hc      *http.Client
	token   TokenSource
}

// New 는 전송기를 만든다.
func New(baseURL string, hc *http.Client, token TokenSource) *Client {
	return &Client{baseURL: baseURL, hc: hc, token: token}
}

// Do 는 요청 한 건을 보내고 본문을 out 에 넣는다.
//
// 401 을 받으면 토큰을 버리고 **한 번만** 다시 시도한다. 그 외에는 재시도하지 않는다 —
// 키움의 429/5xx 정책을 모르는 채로 재시도하면 한도를 더 빨리 태운다.
func (c *Client) Do(ctx context.Context, req Request, out any) (Meta, error) {
	meta, err := c.do(ctx, req, out)
	var ae *APIError
	if err != nil && asErr(err, &ae) && ae.StatusCode == http.StatusUnauthorized {
		c.token.Invalidate()
		return c.do(ctx, req, out)
	}
	return meta, err
}

func (c *Client) do(ctx context.Context, req Request, out any) (Meta, error) {
	tok, err := c.token.Token(ctx)
	if err != nil {
		return Meta{}, err
	}

	var buf bytes.Buffer
	if req.Body != nil {
		if err := json.NewEncoder(&buf).Encode(req.Body); err != nil {
			return Meta{}, err
		}
	} else {
		buf.WriteString("{}")
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+req.Path, &buf)
	if err != nil {
		return Meta{}, err
	}
	hreq.Header.Set("Content-Type", "application/json;charset=UTF-8")
	hreq.Header.Set("authorization", "Bearer "+tok)
	hreq.Header.Set("api-id", req.APIID)
	if req.ContYN != "" {
		hreq.Header.Set("cont-yn", req.ContYN)
	}
	if req.NextKey != "" {
		hreq.Header.Set("next-key", req.NextKey)
	}

	resp, err := c.hc.Do(hreq)
	if err != nil {
		return Meta{}, fmt.Errorf("kiwoom: %s 요청 실패: %w", req.APIID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Meta{}, fmt.Errorf("kiwoom: %s 응답 읽기 실패: %w", req.APIID, err)
	}

	meta := Meta{ContYN: resp.Header.Get("cont-yn"), NextKey: resp.Header.Get("next-key")}

	// 업무 오류는 HTTP 200 + return_code 로 온다. 상태코드만 보면 놓친다.
	var env struct {
		ReturnCode int    `json:"return_code"`
		ReturnMsg  string `json:"return_msg"`
	}
	_ = json.Unmarshal(raw, &env)

	if resp.StatusCode != http.StatusOK || env.ReturnCode != 0 {
		return meta, &APIError{
			StatusCode: resp.StatusCode,
			ReturnCode: env.ReturnCode,
			ReturnMsg:  firstNonEmpty(env.ReturnMsg, resp.Status),
			APIID:      req.APIID,
			Body:       peek(raw),
		}
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return meta, &APIError{
				StatusCode: resp.StatusCode, APIID: req.APIID,
				ReturnMsg: "응답 파싱 실패: " + err.Error(), Body: peek(raw),
			}
		}
	}
	return meta, nil
}

func asErr(err error, target **APIError) bool {
	ae, ok := err.(*APIError)
	if ok {
		*target = ae
	}
	return ok
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func peek(b []byte) string {
	if len(b) > bodyPeek {
		return string(b[:bodyPeek])
	}
	return string(b)
}
```

`errors.go` 의 `APIError` 를 별칭으로 바꾼다(정의가 두 곳에 있으면 어긋난다):

```go
// APIError 는 키움 API 호출 실패다. (설명은 internal/transport 참고)
type APIError = transport.APIError
```

`errors.go` 의 `Error()` 메서드 정의는 지운다 — `transport` 쪽에 있다. `IsCode` 는 남긴다.

- [ ] **Step 4: 통과를 확인한다**

Run: `go test ./... -v` · Expected: transport 4개 + 기존 전부 PASS
Run: `go vet ./...` · Expected: 조용

- [ ] **Step 5: 커밋**

```bash
git add internal/transport errors.go
git commit -m "feat: 키움 REST 전송 계층(api-id·연속조회·401 재발급)" -- internal/transport errors.go
```

---

## Task 6: 설정과 루트 클라이언트

**Files:** Create `config.go`, `client.go`, `client_test.go`

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`client_test.go`:

```go
package kiwoom_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

func TestNewClient_기본은_운영도메인(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.BaseURL(); got != "https://api.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClient_모의투자(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithMock())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.BaseURL(); got != "https://mockapi.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClient_키가_없으면_에러(t *testing.T) {
	if _, err := kiwoom.NewClient("", "SK"); err == nil {
		t.Error("appKey 가 비면 에러여야 한다")
	}
	if _, err := kiwoom.NewClient("AK", ""); err == nil {
		t.Error("secretKey 가 비면 에러여야 한다")
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	t.Setenv("KIWOOM_ENV", "mock")

	c, err := kiwoom.NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	if got := c.BaseURL(); got != "https://mockapi.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClientFromEnv_키없음(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "")
	t.Setenv("KIWOOM_SECRET_KEY", "")
	if _, err := kiwoom.NewClientFromEnv(); err == nil {
		t.Error("환경변수가 없으면 에러여야 한다")
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `go test ./... -run TestNewClient`
Expected: 컴파일 실패 — `undefined: kiwoom.NewClient`

- [ ] **Step 3: 구현한다**

`config.go`:

```go
package kiwoom

import (
	"net/http"
	"time"
)

// 도메인. 키움은 운영과 모의투자를 별도 호스트로 가른다.
const (
	ProdBaseURL = "https://api.kiwoom.com"
	MockBaseURL = "https://mockapi.kiwoom.com"
)

const defaultTimeout = 30 * time.Second

type options struct {
	baseURL    string
	timeout    time.Duration
	httpClient *http.Client
}

// Option 은 NewClient 의 functional option.
type Option func(*options)

// WithMock 은 모의투자 도메인을 쓴다.
func WithMock() Option { return func(o *options) { o.baseURL = MockBaseURL } }

// WithBaseURL 은 베이스 URL 을 직접 지정한다(테스트·프록시용).
func WithBaseURL(u string) Option { return func(o *options) { o.baseURL = u } }

// WithTimeout 은 HTTP 타임아웃을 지정한다(기본 30s). WithHTTPClient 를 쓰면 무시된다.
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// WithHTTPClient 는 사용자 정의 *http.Client 를 주입한다(토큰 발급·API 호출 모두 사용).
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.httpClient = c } }
```

`client.go`:

```go
package kiwoom

import (
	"errors"
	"net/http"
	"os"

	"github.com/kenshin579/kiwoom-go/internal/auth"
	"github.com/kenshin579/kiwoom-go/internal/transport"
)

// Client 는 키움 API 클라이언트다. 하위 클라이언트로 각 API 그룹에 접근한다.
type Client struct {
	baseURL string
	http    *transport.Client
}

// NewClient 는 앱키·시크릿키로 클라이언트를 만든다.
func NewClient(appKey, secretKey string, opts ...Option) (*Client, error) {
	if appKey == "" {
		return nil, errors.New("kiwoom: appKey 가 비어 있다")
	}
	if secretKey == "" {
		return nil, errors.New("kiwoom: secretKey 가 비어 있다")
	}

	o := options{baseURL: ProdBaseURL, timeout: defaultTimeout}
	for _, fn := range opts {
		fn(&o)
	}
	hc := o.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: o.timeout}
	}

	tok := auth.New(o.baseURL, appKey, secretKey, hc)
	return &Client{baseURL: o.baseURL, http: transport.New(o.baseURL, hc, tok)}, nil
}

// NewClientFromEnv 는 환경변수로 클라이언트를 만든다.
//
//	KIWOOM_APP_KEY     앱키(필수)
//	KIWOOM_SECRET_KEY  시크릿키(필수)
//	KIWOOM_ENV         "mock" 이면 모의투자. 그 외/미설정이면 운영
func NewClientFromEnv(opts ...Option) (*Client, error) {
	if os.Getenv("KIWOOM_ENV") == "mock" {
		opts = append([]Option{WithMock()}, opts...)
	}
	return NewClient(os.Getenv("KIWOOM_APP_KEY"), os.Getenv("KIWOOM_SECRET_KEY"), opts...)
}

// BaseURL 은 이 클라이언트가 쓰는 베이스 URL 이다.
func (c *Client) BaseURL() string { return c.baseURL }
```

**`KIWOOM_ENV` 를 opts 앞에 넣는 이유**: 사용자가 명시한 `WithBaseURL` 이 환경변수를 이겨야 한다.

- [ ] **Step 4: 통과를 확인한다**

Run: `go test ./... -v` · Expected: 전부 PASS

- [ ] **Step 5: 커밋**

```bash
git add config.go client.go client_test.go
git commit -m "feat: 클라이언트 설정과 환경변수 로딩" -- config.go client.go client_test.go
```

---

## Task 7: tools 모듈과 스펙 벤더링

**Files:** Create `tools/go.mod`, `tools/spec/kiwoom_api_spec.json`, `tools/spec/api_names.json`, `tools/spec/SOURCE.md`

- [ ] **Step 1: 모듈을 만들고 스펙을 받는다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
mkdir -p tools/spec tools/gen
cd tools && go mod init github.com/kenshin579/kiwoom-go/tools && cd ..

curl -sL -o tools/spec/kiwoom_api_spec.json \
  https://raw.githubusercontent.com/Kiwoom-Securities/Kiwoom-REST-API/main/kiwoom/_data/kiwoom_api_spec.json
```

받은 커밋을 기록한다:

```bash
gh api repos/Kiwoom-Securities/Kiwoom-REST-API/commits/main --jq '.sha' > /tmp/kiwoom_sha
```

`tools/spec/SOURCE.md` 에 출처를 적는다(위 SHA 를 넣을 것):

```markdown
# 스펙 출처

- 저장소: https://github.com/Kiwoom-Securities/Kiwoom-REST-API
- 파일: `kiwoom/_data/kiwoom_api_spec.json`
- 받은 커밋: <위에서 얻은 SHA>
- 받은 날짜: 2026-09-14

갱신하려면 같은 경로에서 다시 받고 이 파일의 커밋·날짜를 고친 뒤 `go run ./gen` 을 다시 돌린다.
```

- [ ] **Step 2: 이름표를 뽑는다**

예제 362개의 머리말에서 `api_id` 와 파일명을 모아 `api-id → 공식 영문명` 표를 만든다.
**한 번만 돌리는 작업**이므로 스크립트를 저장소에 남기지 않고, 결과 JSON 만 벤더링한다.

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
gh api "repos/Kiwoom-Securities/Kiwoom-REST-API/git/trees/main?recursive=1" \
  --jq '.tree[] | select(.type=="blob") | select(.path|startswith("examples/")) | select(.path|endswith(".py")) | .path' \
  > /tmp/example_paths.txt
wc -l < /tmp/example_paths.txt   # 362 근처여야 한다

python3 - <<'PY' > tools/spec/api_names.json
import json, os, subprocess, urllib.parse, urllib.request
names = {}
for path in open('/tmp/example_paths.txt').read().splitlines():
    url = "https://raw.githubusercontent.com/Kiwoom-Securities/Kiwoom-REST-API/main/" + urllib.parse.quote(path)
    head = urllib.request.urlopen(url).read(400).decode('utf-8', 'replace')
    api_id = ""
    for line in head.splitlines():
        if line.startswith('# api_id:'):
            api_id = line.split(':', 1)[1].strip()
            break
    if api_id:
        names[api_id] = os.path.basename(path)[:-3]
print(json.dumps(names, ensure_ascii=False, indent=2, sort_keys=True))
PY

jq 'length' tools/spec/api_names.json   # 330 이상이어야 한다
```

**이번 범위 77개가 전부 들어 있는지 확인한다:**

```bash
jq -r '.apis | to_entries[]
  | select(.value.meta["메뉴 위치"] // "" | test("^국내주식 > (시세|차트|종목정보)"))
  | .value.meta["API ID"]' tools/spec/kiwoom_api_spec.json | sort > /tmp/target_ids.txt
wc -l < /tmp/target_ids.txt   # 77 이어야 한다
comm -23 /tmp/target_ids.txt <(jq -r 'keys[]' tools/spec/api_names.json | sort)
# 출력이 비어야 한다 — 비지 않으면 이름표가 없는 API 가 있다는 뜻이므로 보고할 것
```

- [ ] **Step 3: 라이브러리 모듈에서 제외됐는지 확인한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
go list ./... | grep tools
```
Expected: **출력 없음** — `tools/go.mod` 가 있으면 부모 모듈이 그 하위를 제외한다.
출력이 있으면 `tools/go.mod` 가 없는 것이므로 멈추고 보고하라.

- [ ] **Step 4: 커밋**

```bash
git add tools/go.mod tools/spec
git commit -m "chore: 키움 공식 스펙과 이름표 벤더링" -- tools/go.mod tools/spec
```

---

## Task 8: 생성기 — 스펙을 트리로

**Files:** Create `tools/gen/spec.go`, `tools/gen/spec_test.go`

**여기가 생성기의 핵심이다.** 스펙의 응답 필드는 평평한 배열이고 `depth` 로 중첩을 나타낸다.
**최대 3단계(depth 0·1·2)** 이고 depth 2 필드가 611개 있으므로 한 단계만 다루면 틀린다.

규칙: `type` 이 `LIST` 인 필드는 뒤따르는 **더 깊은** 필드들을 자식으로 갖는다. 자기와 같거나
얕은 깊이가 나오면 끝난다.

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`tools/gen/spec_test.go`:

```go
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
```

- [ ] **Step 2: 실패를 확인한다**

Run: `cd tools && go test ./gen/ -v`
Expected: 컴파일 실패 — `undefined: gen.BuildTree`

- [ ] **Step 3: 구현한다**

`tools/gen/spec.go`:

```go
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
// 응답은 최대 3단계(0·1·2)라 재귀가 필요하다.
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
```

- [ ] **Step 4: 통과를 확인한다**

Run: `cd tools && go test ./gen/ -v`
Expected: 3개 PASS

- [ ] **Step 5: 커밋**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
git add tools/gen/spec.go tools/gen/spec_test.go tools/go.sum
git commit -m "feat(gen): 스펙 파싱과 depth 트리 변환" -- tools/gen/spec.go tools/gen/spec_test.go tools/go.sum
```

---

## Task 9: 생성기 — 이름 변환과 렌더링

**Files:** Create `tools/gen/name.go`, `tools/gen/name_test.go`, `tools/gen/render.go`, `tools/gen/render_test.go`

- [ ] **Step 1: 이름 변환 테스트를 쓴다**

`tools/gen/name_test.go`:

```go
package gen_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func TestGoName(t *testing.T) {
	cases := map[string]string{
		"get_domestic_stock_quote":    "GetDomesticStockQuote",
		"get_domestic_elw_detail":     "GetDomesticElwDetail", // 도메인 약어는 올리지 않는다
		"bid_req_base_tm":             "BidReqBaseTm",
		"stk_cd":                      "StkCd",
		"sel_10th_pre_req_pre":        "Sel10thPreReqPre", // 숫자로 시작하는 조각
		"api_id":                      "APIID",            // 린트가 문제 삼는 것만 올린다
		"item_url":                    "ItemURL",
		"http_status":                 "HTTPStatus",
		"":                            "",
	}
	for in, want := range cases {
		if got := gen.GoName(in); got != want {
			t.Errorf("GoName(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: 실패를 확인한다**

Run: `cd tools && go test ./gen/ -run TestGoName`
Expected: 컴파일 실패 — `undefined: gen.GoName`

- [ ] **Step 3: 이름 변환을 구현한다**

`tools/gen/name.go`:

```go
package gen

import "strings"

// initialisms 는 Go 린트가 문제 삼는 약어만 담는다.
//
// elw·etf 같은 도메인 약어는 일부러 넣지 않는다 — 목록을 키우면 "예외 없는 규칙" 이라는
// 이 생성기의 전제가 무너지고, 사용자가 문서에서 본 이름을 그대로 추측하지 못하게 된다.
var initialisms = map[string]string{
	"api":  "API",
	"id":   "ID",
	"url":  "URL",
	"http": "HTTP",
}

// GoName 은 snake_case 를 Go 식별자로 바꾼다.
func GoName(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		if up, ok := initialisms[part]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}
```

- [ ] **Step 4: 렌더링 골든 테스트를 쓴다**

`tools/gen/render_test.go`:

```go
package gen_test

import (
	"strings"
	"testing"

	"github.com/kenshin579/kiwoom-go/tools/gen"
)

func TestRender(t *testing.T) {
	api := gen.Target{
		Package:  "quote",
		GoName:   "GetDomesticStockQuote",
		APIID:    "ka10004",
		APIName:  "주식호가요청",
		MenuPath: "국내주식 > 시세 > 주식호가요청(ka10004)",
		Path:     "/api/dostk/mrkcond",
		Request: []gen.Node{
			{Field: gen.Field{Element: "stk_cd", Korean: "종목코드", Required: "Y", Length: "20"}},
		},
		Response: []gen.Node{
			{Field: gen.Field{Element: "bid_req_base_tm", Korean: "호가잔량기준시간"}},
			{Field: gen.Field{Element: "quotes", Korean: "호가목록", Type: "LIST"}, IsList: true, Children: []gen.Node{
				{Field: gen.Field{Element: "dt", Korean: "일자", Description: "YYYYMMDD"}},
			}},
		},
	}

	src, err := gen.Render(api)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := string(src)

	for _, want := range []string{
		"// Code generated by tools/gen. DO NOT EDIT.",
		"package quote",
		"// GetDomesticStockQuote 는 주식호가요청(ka10004) 이다.",
		"type GetDomesticStockQuoteRequest struct {",
		"StkCd string `json:\"stk_cd\"`",
		"// 종목코드 (필수, 20자)",
		"type GetDomesticStockQuoteResponse struct {",
		"Quotes []GetDomesticStockQuoteQuotesItem `json:\"quotes\"`",
		"type GetDomesticStockQuoteQuotesItem struct {",
		"// 일자 (YYYYMMDD)",
		"func (c *Client) GetDomesticStockQuote(ctx context.Context",
		`APIID: "ka10004"`,
		`Path:  "/api/dostk/mrkcond"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("생성 결과에 %q 가 없다\n---\n%s", want, got)
		}
	}
}

func TestRender_gofmt된다(t *testing.T) {
	// Render 가 go/format 을 통과시키므로, 문법이 깨지면 여기서 에러가 난다.
	if _, err := gen.Render(gen.Target{Package: "quote", GoName: "X", APIID: "ka1", Path: "/p"}); err != nil {
		t.Fatalf("빈 API 도 컴파일 가능한 코드여야 한다: %v", err)
	}
}
```

- [ ] **Step 5: 실패를 확인한다**

Run: `cd tools && go test ./gen/ -run TestRender`
Expected: 컴파일 실패 — `undefined: gen.Target`

- [ ] **Step 6: 렌더링을 구현한다**

`tools/gen/render.go`:

```go
package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"text/template"
)

// Target 은 파일 하나로 생성할 API.
type Target struct {
	Package  string
	GoName   string
	APIID    string
	APIName  string
	MenuPath string
	Path     string
	Request  []Node
	Response []Node
}

// Render 는 Target 을 gofmt 된 Go 소스로 만든다.
func Render(t Target) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t); err != nil {
		return nil, err
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt 실패(%s): %w\n%s", t.GoName, err, buf.String())
	}
	return src, nil
}

// structs 는 노드 목록을 [구조체 본문, LIST 가 만드는 추가 구조체들] 로 펼친다.
//
// 슬라이스로 돌려주는 이유: text/template 의 FuncMap 은 다중 반환을 받지 못한다.
// 템플릿에서 `index $x 0` / `index $x 1` 로 꺼낸다.
func structs(prefix string, ns []Node) []string {
	var b, x strings.Builder
	for _, n := range ns {
		name := GoName(n.Element)
		if c := comment(n); c != "" {
			fmt.Fprintf(&b, "\t// %s\n", c)
		}
		if n.IsList {
			item := prefix + name + "Item"
			fmt.Fprintf(&b, "\t%s []%s `json:%q`\n", name, item, n.Element)
			inner, deeper := structs(item, n.Children)
			fmt.Fprintf(&x, "\n// %s 는 %s 의 원소다.\ntype %s struct {\n%s}\n", item, name, item, inner)
			x.WriteString(deeper)
			continue
		}
		fmt.Fprintf(&b, "\t%s string `json:%q`\n", name, n.Element)
	}
	return []string{b.String(), x.String()}
}

// comment 는 한글명·필수·길이·설명을 한 줄로 만든다.
func comment(n Node) string {
	var bits []string
	if n.Required == "Y" {
		bits = append(bits, "필수")
	}
	if n.Length != "" {
		bits = append(bits, n.Length+"자")
	}
	if n.Description != "" {
		bits = append(bits, n.Description)
	}
	if len(bits) == 0 {
		return n.Korean
	}
	if n.Korean == "" {
		return strings.Join(bits, ", ")
	}
	return fmt.Sprintf("%s (%s)", n.Korean, strings.Join(bits, ", "))
}

var tmpl = template.Must(template.New("api").Funcs(template.FuncMap{
	"structs": structs,
}).Parse(`// Code generated by tools/gen. DO NOT EDIT.

package {{.Package}}

import (
	"context"

	"github.com/kenshin579/kiwoom-go/internal/transport"
)

{{- $req := structs (printf "%sRequest" .GoName) .Request}}
{{- $res := structs (printf "%s" .GoName) .Response}}

// {{.GoName}}Request 는 {{.APIName}}({{.APIID}}) 요청이다.
type {{.GoName}}Request struct {
{{index $req 0}}}

// {{.GoName}}Response 는 {{.APIName}}({{.APIID}}) 응답이다.
type {{.GoName}}Response struct {
{{index $res 0}}}
{{index $res 1}}
// {{.GoName}} 는 {{.APIName}}({{.APIID}}) 이다.
//
// 메뉴: {{.MenuPath}}
// URL:  {{.Path}}
func (c *Client) {{.GoName}}(ctx context.Context, req {{.GoName}}Request, opts ...transport.Option) (*{{.GoName}}Response, transport.Meta, error) {
	var out {{.GoName}}Response
	meta, err := c.http.Do(ctx, transport.Request{
		APIID: "{{.APIID}}",
		Path:  "{{.Path}}",
		Body:  req,
	}, &out)
	if err != nil {
		return nil, meta, err
	}
	return &out, meta, nil
}
`))
```

`transport.Option` 은 연속조회를 넘기기 위한 것이다. `internal/transport` 에 추가한다:

```go
// Option 은 호출 단위 옵션.
type Option func(*Request)

// WithCont 는 연속조회를 이어간다. 직전 응답의 Meta 를 그대로 넘긴다.
func WithCont(m Meta) Option {
	return func(r *Request) { r.ContYN, r.NextKey = m.ContYN, m.NextKey }
}
```

그리고 `Do` 의 시그니처를 `Do(ctx, req Request, out any, opts ...Option)` 으로 바꿔
맨 앞에서 `for _, fn := range opts { fn(&req) }` 를 돌린다. 기존 transport 테스트는
옵션을 넘기지 않으므로 그대로 통과한다.

- [ ] **Step 7: 통과를 확인한다**

Run: `cd tools && go test ./gen/ -v` · Expected: 이름·렌더 테스트 전부 PASS
Run: `cd .. && go test ./... && go vet ./...` · Expected: 루트 모듈도 그대로 통과

- [ ] **Step 8: 커밋**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
git add tools/gen internal/transport
git commit -m "feat(gen): 이름 변환과 코드 렌더링" -- tools/gen internal/transport
```

---

## Task 10: 생성기 실행과 77개 생성

**Files:** Create `tools/gen/main/main.go`, `domestic/{quote,chart,stock}/client.go`, 생성물 77개

- [ ] **Step 1: 하위 클라이언트를 손으로 쓴다**

세 파일이 같은 모양이다. `domestic/quote/client.go`:

```go
// Package quote 는 키움 국내주식 시세 API 그룹이다.
package quote

import "github.com/kenshin579/kiwoom-go/internal/transport"

// Client 는 시세 하위 클라이언트.
type Client struct {
	http *transport.Client
}

// New 는 kiwoom.NewClient 가 호출한다.
func New(hc *transport.Client) *Client { return &Client{http: hc} }
```

`domestic/chart/client.go` 는 `package chart` + `차트`, `domestic/stock/client.go` 는
`package stock` + `종목정보` 로 같은 내용을 쓴다.

- [ ] **Step 2: 생성기 진입점을 쓴다**

`tools/gen/main/main.go`:

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

	for _, api := range spec.APIs {
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

		target := gen.Target{
			Package:  pkg,
			GoName:   gen.GoName(name),
			APIID:    id,
			APIName:  api.Meta["API 명"],
			MenuPath: menu,
			Path:     api.Meta["URL"],
			Request:  gen.BuildTree(api.Request.Body),
			Response: gen.BuildTree(api.Response.Body),
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

	for pkg, n := range counts {
		fmt.Printf("%s: %d개\n", pkg, n)
	}
	if len(missing) > 0 {
		// 이름표가 없으면 멈춘다. api-id 로 대충 이름을 지으면 나중에 바꿀 수 없다.
		log.Fatalf("이름표 없는 API %d개: %v", len(missing), missing)
	}
}
```

- [ ] **Step 3: 생성한다**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go/tools
go run ./gen/main
```
Expected: `quote: 25개` · `chart: 21개` · `stock: 31개` (합 77). 다른 숫자면 멈추고 보고하라.

- [ ] **Step 4: 컴파일과 검사**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
gofmt -l domestic/      # 출력 없어야 한다
go build ./... && go vet ./...
go test ./...
ls domestic/quote/*.go | wc -l   # 26 (생성 25 + client.go)
```

**컴파일이 깨지면 생성기를 고쳐 다시 돌린다. 생성물을 손으로 고치지 마라** —
`DO NOT EDIT` 파일이고 다음 생성 때 지워진다.

- [ ] **Step 5: 루트 클라이언트에 연결한다**

`client.go` 의 `Client` 에 필드를 더하고 `NewClient` 에서 채운다:

```go
// Client 는 키움 API 클라이언트다. 하위 클라이언트로 각 API 그룹에 접근한다.
type Client struct {
	baseURL string
	http    *transport.Client

	// DomesticQuote 는 국내주식 시세.
	DomesticQuote *quote.Client
	// DomesticChart 는 국내주식 차트.
	DomesticChart *chart.Client
	// DomesticStock 은 국내주식 종목정보.
	DomesticStock *stock.Client
}
```

`NewClient` 의 마지막 부분:

```go
	tr := transport.New(o.baseURL, hc, tok)
	return &Client{
		baseURL:       o.baseURL,
		http:          tr,
		DomesticQuote: quote.New(tr),
		DomesticChart: chart.New(tr),
		DomesticStock: stock.New(tr),
	}, nil
```

import 에 세 패키지를 추가한다.

- [ ] **Step 6: 생성 결과가 스펙과 맞는지 검증한다**

`domestic/quote/generated_test.go`:

```go
package quote_test

import (
	"reflect"
	"testing"

	"github.com/kenshin579/kiwoom-go/domestic/quote"
)

// 생성물이 실제로 쓸 수 있는 모양인지 확인한다.
// 스펙 전체를 다시 검증하지는 않는다 — 그건 생성기 테스트의 몫이다.
func TestGeneratedShape(t *testing.T) {
	var req quote.GetDomesticStockQuoteRequest
	rt := reflect.TypeOf(req)
	f, ok := rt.FieldByName("StkCd")
	if !ok {
		t.Fatal("StkCd 필드가 없다")
	}
	if got := f.Tag.Get("json"); got != "stk_cd" {
		t.Errorf("json 태그 = %q, want stk_cd", got)
	}
	if f.Type.Kind() != reflect.String {
		t.Errorf("필드 타입 = %v, want string (모든 필드는 문자열이다)", f.Type.Kind())
	}

	var resp quote.GetDomesticStockQuoteResponse
	if reflect.TypeOf(resp).NumField() == 0 {
		t.Error("응답 필드가 비어 있다")
	}
}
```

Run: `go test ./domestic/...` · Expected: PASS

- [ ] **Step 7: 커밋**

생성물이 많으므로 두 커밋으로 나눈다.

```bash
git add tools/gen/main domestic/quote/client.go domestic/chart/client.go domestic/stock/client.go client.go
git commit -m "feat: 생성기 진입점과 하위 클라이언트" -- tools/gen/main domestic/quote/client.go domestic/chart/client.go domestic/stock/client.go client.go

git add domestic
git commit -m "feat: 국내주식 시세·차트·종목정보 77개 생성" -- domestic
```

---

## Task 11: README 와 최종 검증

**Files:** Modify `README.md`

- [ ] **Step 1: README 를 쓴다**

`README.md` 를 형제 저장소(`toss-go`) 형식으로 교체한다. 반드시 담을 것:

- 한 줄 소개와 **지원 범위** — 이번엔 국내주식 시세 25 · 차트 21 · 종목정보 31 = 77개이고,
  나머지 260개는 아직이라는 것
- 설치(`go get github.com/kenshin579/kiwoom-go@latest`), Go 1.25
- 인증 — `KIWOOM_APP_KEY`·`KIWOOM_SECRET_KEY`·`KIWOOM_ENV`, `NewClient`/`NewClientFromEnv`,
  모의투자 지원
- **첫 호출 예제** — `c.DomesticQuote.GetDomesticStockQuote(ctx, ...)`
- **문자열 정책** — 응답 필드는 전부 `string` 이고 `kiwoom.Decimal`·`kiwoom.Date` 로 바꾼다는
  것, 빈 문자열이 "값 없음" 이라는 것
- **연속조회** — `transport.Meta` 의 `ContYN`/`NextKey` 를 다음 호출에 넘긴다는 것
- **재시도를 넣지 않았다**는 것과 그 이유(키움 429/5xx 정책 미확인)
- 코드 생성 — `cd tools && go run ./gen/main`, 스펙 출처는 `tools/spec/SOURCE.md`
- **`v0.1.0` 이 보증하는 범위** — "생성되고 컴파일되며 스펙과 일치한다" 까지이고 실 API
  호출로는 검증되지 않았다는 것
- MIT 라이선스

- [ ] **Step 2: 전체 검증**

```bash
cd /Users/frankoh/src/workspace_moneyflow/kiwoom-go
gofmt -l . | grep -v '^tools/spec' || true   # 출력 없어야 한다
go build ./... && go vet ./... && go test ./...
cd tools && go build ./... && go vet ./... && go test ./... && cd ..
go list ./... | grep tools && echo "❌ tools 가 라이브러리 모듈에 포함됐다" || echo "✅ tools 제외됨"
```

- [ ] **Step 3: 커밋과 PR**

```bash
git add README.md
git commit -m "docs: README — 지원 범위·인증·문자열 정책·연속조회" -- README.md

git push -u origin feature/core-and-generator
gh pr create --base main --title "feat: 코어와 스펙 기반 생성기, 국내주식 77개" --body "$(cat <<'EOF'
## 무엇

키움증권 REST API 의 Go 클라이언트 뼈대와 **스펙 기반 코드 생성기**를 만들고, 국내주식
시세 25 · 차트 21 · 종목정보 31 = **77개**를 생성했다.

## 왜 생성기인가

키움 API 는 **337개**다. `toss-go`(30~40개, 9,600줄)처럼 손으로 쓰면 완주할 수 없다.
공식 저장소가 기계 판독 스펙(`kiwoom_api_spec.json`)을 주므로 거기서 만든다.

전송이 거의 공짜라는 점이 이를 뒷받침한다 — **337개가 전부 POST 이고 URL 은 29개뿐이며
구분은 `api-id` 헤더**다. 연속조회도 전부 같다. 함수 하나가 전부를 실어 나르고, 생성되는
코드는 얇은 껍데기다.

## 설계 판단

- **응답 필드는 전부 `string`.** 스펙상 5,500개 중 5,500개가 String 이고 타입 힌트는 한글
  설명뿐이다. 설명을 읽어 타입을 추론하면 **틀린 값을 맞는 값처럼 보여주는** 실패가 된다.
  대신 `kiwoom.Decimal`·`kiwoom.Date` 헬퍼가 부호(`"+1234"`)·앞자리 0·콤마·빈 값을 한 곳에서 처리한다.
- **이름은 지어내지 않는다.** 공식 예제 362개의 머리말에서 `api-id → 영문명` 을 뽑아
  `GetDomesticStockQuote` 처럼 그대로 옮긴다. 패키지명과 겹쳐 말을 더듬지만, 337개를 기계가
  찍어내는 라이브러리에서는 예측 가능성이 미감보다 값어치가 크다.
- **생성기와 3.7MB 스펙은 `tools/` 별도 모듈.** 같은 모듈이면 라이브러리를 `go get` 하는
  모든 사람이 쓰지도 않는 JSON 을 내려받는다.
- **재시도를 넣지 않았다.** 키움의 429/5xx 정책을 확인하기 전에 추측으로 넣으면 한도를 더
  빨리 태운다. 401 토큰 만료만 1회 재발급한다.

## 검증

- `go build` · `go vet` · `go test` — 루트·tools 두 모듈 모두 통과
- 생성물 77개가 컴파일되고 `gofmt` 를 통과
- `go list ./...` 에 `tools` 가 없다(모듈 제외 확인)

**실 API 호출로는 검증되지 않았다.** `v0.1.0` 이 보증하는 것은 "생성되고 컴파일되며 스펙과
일치한다" 까지다. README 에 그대로 적었다.

## 다음

- 나머지 REST ~260개 (계좌·주문·순위정보·미국주식·ELW·ETF)
- 실시간 WebSocket 23 + 조건검색 8

설계: `docs/superpowers/specs/2026-09-14-kiwoom-go-core-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)

https://claude.ai/code/session_015hDGfcBaEbCXvTCHfwS5ME
EOF
)"
```

---

## 자체 검토 메모

- 설계 §5 구조 → Task 1·5·6·10. §6 생성기 → Task 7·8·9·10. §6.1 이름 규칙 → Task 9.
  §7 문자열 정책 → Task 2. §8 인증·에러 → Task 3·4·5. §9 테스트 → 각 Task 의 테스트 단계.
  §10 공개 채비 → Task 1(LICENSE)·Task 11(README). §11 위험 → Task 7 Step 2(이름표 누락 검증),
  Task 10 Step 3(개수 검증), Task 11(보증 범위 명시).
- **설계 §9 의 "각 요청 구조체의 필수 필드가 스펙과 일치하는지 표 검증" 을 줄였다.** Task 10
  Step 6 은 대표 한 건만 확인한다. 77개를 전수 대조하는 테스트는 생성기를 두 번 구현하는
  꼴이라 값어치보다 비용이 크다 — 생성기 골든 테스트(Task 8·9)가 규칙을 덮는다.
- **`/oauth2/revoke`(접근토큰폐기)는 넣지 않았다.** 설계 §8 이 발급만 적었고, 토큰은 만료로
  정리된다. 필요해지면 한 함수다.
- 이름 일관성: `transport.Client`·`transport.Request`·`transport.Meta`·`transport.Option`(Task 5·9),
  `auth.Source`(Task 4), `gen.Field`·`gen.Node`·`gen.BuildTree`·`gen.GoName`·`gen.Target`·
  `gen.Render`(Task 8·9), `kiwoom.NewClient`·`NewClientFromEnv`·`WithMock`(Task 6).
- `structs` 는 처음부터 `[]string` 을 돌려준다 — `text/template` 의 FuncMap 이 다중 반환을
  받지 못하기 때문이다. 템플릿은 `index $x 0`(구조체 본문)·`index $x 1`(추가 구조체)로 꺼낸다.
