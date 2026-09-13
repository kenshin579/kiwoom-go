package kiwoom

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kenshin579/kiwoom-go/internal/auth"
	"github.com/kenshin579/kiwoom-go/internal/transport"
)

// Client 는 키움 API 클라이언트다.
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
	// WithBaseURL("") 이 요청을 상대 경로로 만들지 않도록 되돌린다(toss-go 와 같은 가드).
	if o.baseURL == "" {
		o.baseURL = ProdBaseURL
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
//	KIWOOM_ENV         "mock" 이면 모의투자, "prod" 또는 미설정이면 운영(대소문자 무시)
//
// 알 수 없는 KIWOOM_ENV 값은 **에러**다. 오타(`"moc"`)를 조용히 운영으로 떨어뜨리면
// 사용자가 모의투자로 믿고 실거래 서버를 치게 된다 — 시작하지 못하는 편이 낫다.
//
// 명시한 opts 가 환경변수를 이긴다 — 환경변수 옵션을 맨 앞에 넣기 때문이다.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	switch env := strings.ToLower(strings.TrimSpace(os.Getenv("KIWOOM_ENV"))); env {
	case "", "prod":
		// 운영(기본)
	case "mock":
		opts = append([]Option{WithMock()}, opts...)
	default:
		return nil, fmt.Errorf("kiwoom: KIWOOM_ENV 값을 알 수 없다: %q (mock 또는 prod)", env)
	}
	return NewClient(os.Getenv("KIWOOM_APP_KEY"), os.Getenv("KIWOOM_SECRET_KEY"), opts...)
}

// BaseURL 은 이 클라이언트가 쓰는 베이스 URL 이다.
func (c *Client) BaseURL() string { return c.baseURL }
