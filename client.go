package kiwoom

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kenshin579/kiwoom-go/internal/auth"
	"github.com/kenshin579/kiwoom-go/internal/transport"
	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

// Client 는 키움 API 클라이언트다. 하위 클라이언트로 각 API 그룹에 접근한다.
//
// Clients 를 임베딩하므로 c.DomesticQuote 처럼 바로 쓴다. 그 목록은 생성물
// subclients.go 에 있다.
//
// 다 썼으면 Close 를 불러라 — WebSocket 을 쓴 경우 소켓과 재연결 루프가 남는다.
type Client struct {
	baseURL string

	// dom·ovs 는 시장별 WebSocket 연결이다. Clients 안의 실시간·조건검색 하위
	// 클라이언트가 같은 것을 들고 있지만, 그쪽은 생성물의 비공개 필드라 여기서 닿지
	// 못한다 — Close 와 BadFrames 가 연결에 닿으려면 루트가 직접 들어야 한다.
	dom *wstransport.Conn
	ovs *wstransport.Conn

	Clients
}

// NewClient 는 앱키·시크릿키로 클라이언트를 만든다.
func NewClient(appKey, secretKey string, opts ...Option) (*Client, error) {
	if appKey == "" {
		return nil, errors.New("kiwoom: appKey 가 비어 있다")
	}
	if secretKey == "" {
		return nil, errors.New("kiwoom: secretKey 가 비어 있다")
	}

	o := options{baseURL: ProdBaseURL, wsBaseURL: ProdWSURL, timeout: defaultTimeout}
	for _, fn := range opts {
		fn(&o)
	}
	// WithBaseURL("") 이 요청을 상대 경로로 만들지 않도록 되돌린다(toss-go 와 같은 가드).
	if o.baseURL == "" {
		o.baseURL = ProdBaseURL
	}
	// WS 쪽도 같은 이유로 되돌린다 — 빈 값이면 다이얼 주소가 경로만 남는다.
	if o.wsBaseURL == "" {
		o.wsBaseURL = ProdWSURL
	}
	hc := o.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: o.timeout}
	}

	tok := auth.New(o.baseURL, appKey, secretKey, hc)
	tr := transport.New(o.baseURL, hc, tok)
	// WebSocket 은 시장마다 연결이 하나씩이다(국내·미국). 아직 다이얼하지 않는다 —
	// 첫 구독 때 붙는다.
	dom := wstransport.New(o.wsBaseURL, wstransport.DomesticPath, tok)
	ovs := wstransport.New(o.wsBaseURL, wstransport.OverseasPath, tok)
	return &Client{
		baseURL: o.baseURL,
		dom:     dom,
		ovs:     ovs,
		Clients: newClients(tr, dom, ovs),
	}, nil
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

// Close 는 WebSocket 연결(국내·미국)을 닫는다. 두 번 불러도 안전하다.
//
// REST 만 쓴 클라이언트도 불러도 된다 — 연결은 첫 구독·첫 조건검색 때 붙으므로
// 붙은 적이 없으면 닫을 것도 없다.
//
// 이것이 없으면 사용자가 소켓을 닫을 방법이 없다. 연결은 internal/ 안에 살고,
// 구독 ctx 를 취소해도 채널만 닫힐 뿐 소켓과 재연결 루프는 남는다.
//
// 한쪽이 실패해도 **다른 쪽을 반드시 닫는다.** 먼저 실패한 곳에서 빠져나오면 나머지
// 하나가 열린 채 남는다. 에러는 errors.Join 으로 합쳐 둘 다 보여준다.
func (c *Client) Close() error {
	return errors.Join(c.dom.Close(), c.ovs.Close())
}

// BadFrames 는 WebSocket 에서 **해석하지 못해 버린** 메시지 수다(국내·미국 연결의 합).
//
// 깨진 JSON 한 건으로는 재연결하지 않는다 — 재연결이 되찾는 것이 없고, 서버가 우리가
// 모르는 메시지를 계속 보내면 재연결 핫 루프가 되기 때문이다. 대신 세어서 여기로 낸다.
//
// 그러니 이 수가 0 이 아니고 계속 는다면, 조용히 버려지고 있는 실시간이 있다는 뜻이다.
// "왜 어떤 이벤트는 안 오지?" 를 설명할 수 있는 유일한 자리다.
func (c *Client) BadFrames() int {
	return c.dom.BadFrames() + c.ovs.BadFrames()
}
