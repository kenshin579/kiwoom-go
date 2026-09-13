package kiwoom

import (
	"errors"
	"net/http"
	"os"

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
//
// 명시한 opts 가 환경변수를 이긴다 — 환경변수 옵션을 맨 앞에 넣기 때문이다.
func NewClientFromEnv(opts ...Option) (*Client, error) {
	if os.Getenv("KIWOOM_ENV") == "mock" {
		opts = append([]Option{WithMock()}, opts...)
	}
	return NewClient(os.Getenv("KIWOOM_APP_KEY"), os.Getenv("KIWOOM_SECRET_KEY"), opts...)
}

// BaseURL 은 이 클라이언트가 쓰는 베이스 URL 이다.
func (c *Client) BaseURL() string { return c.baseURL }
