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
//
// WithBaseURL 과 함께 주면 **나중에 준 것이 이긴다**(functional option 의 일반 규칙).
func WithMock() Option { return func(o *options) { o.baseURL = MockBaseURL } }

// WithBaseURL 은 베이스 URL 을 직접 지정한다(테스트·프록시용).
//
// 토큰 발급(`POST {baseURL}/oauth2/token`)도 이 URL 로 간다 — API 트래픽만 프록시하려던
// 사용자가 앱키·시크릿키까지 그리로 보내게 되므로 주의할 것.
func WithBaseURL(u string) Option { return func(o *options) { o.baseURL = u } }

// WithTimeout 은 HTTP 타임아웃을 지정한다(기본 30s). WithHTTPClient 를 쓰면 무시된다.
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// WithHTTPClient 는 사용자 정의 *http.Client 를 주입한다(토큰 발급·API 호출 모두 사용).
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.httpClient = c } }
