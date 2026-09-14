package kiwoom

import (
	"net/http"
	"time"

	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

// 도메인. 키움은 운영과 모의투자를 별도 호스트로 가른다.
const (
	ProdBaseURL = "https://api.kiwoom.com"
	MockBaseURL = "https://mockapi.kiwoom.com"
)

// WebSocket 도메인. REST 와 달리 포트가 붙는다.
const (
	ProdWSURL = wstransport.ProdURL
	MockWSURL = wstransport.MockURL
)

const defaultTimeout = 30 * time.Second

type options struct {
	baseURL    string
	wsBaseURL  string
	timeout    time.Duration
	httpClient *http.Client
}

// Option 은 NewClient 의 functional option.
type Option func(*options)

// WithMock 은 모의투자 도메인을 쓴다(REST·WebSocket 둘 다).
//
// 둘을 함께 바꾸는 이유: 하나만 바뀌면 사용자가 모의투자로 믿는 채로 실서버에 붙는다.
//
// WithBaseURL 과 함께 주면 **나중에 준 것이 이긴다**(functional option 의 일반 규칙).
func WithMock() Option {
	return func(o *options) { o.baseURL, o.wsBaseURL = MockBaseURL, MockWSURL }
}

// WithBaseURL 은 REST 베이스 URL 을 직접 지정한다(테스트·프록시용).
//
// WebSocket 은 바뀌지 않는다 — 그쪽은 WithWSBaseURL 이다.
//
// 토큰 발급(`POST {baseURL}/oauth2/token`)도 이 URL 로 간다 — API 트래픽만 프록시하려던
// 사용자가 앱키·시크릿키까지 그리로 보내게 되므로 주의할 것.
func WithBaseURL(u string) Option { return func(o *options) { o.baseURL = u } }

// WithWSBaseURL 은 WebSocket 베이스 URL 을 직접 지정한다(테스트·프록시용).
//
// WithBaseURL 은 REST 만 바꾼다 — 두 주소는 호스트도 포트도 다르므로 하나로 묶으면
// REST 프록시를 붙이려던 사용자의 WebSocket 이 조용히 엉뚱한 곳으로 간다.
func WithWSBaseURL(u string) Option { return func(o *options) { o.wsBaseURL = u } }

// WithTimeout 은 HTTP 타임아웃을 지정한다(기본 30s). WithHTTPClient 를 쓰면 무시된다.
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// WithHTTPClient 는 사용자 정의 *http.Client 를 주입한다(토큰 발급·API 호출 모두 사용).
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.httpClient = c } }
