package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// bodyPeek 은 실패 시 에러에 담을 원문 길이.
const bodyPeek = 512

// TokenSource 는 접근토큰을 준다. internal/auth.Source 가 만족한다.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	Invalidate()
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

// Option 은 호출 단위 옵션.
type Option func(*Request)

// WithCont 는 연속조회를 이어간다. 직전 응답의 Meta 를 그대로 넘긴다.
func WithCont(m Meta) Option {
	return func(r *Request) { r.ContYN, r.NextKey = m.ContYN, m.NextKey }
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

// Do 는 요청 한 건을 보내고 본문을 out 에 넣는다. out 이 nil 이면 본문을 버린다.
//
// 401 을 받으면 토큰을 버리고 **한 번만** 다시 시도한다. 그 외에는 재시도하지 않는다 —
// 키움의 429/5xx 정책을 모르는 채로 재시도하면 한도를 더 빨리 태운다.
func (c *Client) Do(ctx context.Context, req Request, out any, opts ...Option) (Meta, error) {
	for _, fn := range opts {
		fn(&req)
	}
	meta, err := c.do(ctx, req, out)
	var ae *APIError
	if err != nil && errors.As(err, &ae) && ae.StatusCode == http.StatusUnauthorized {
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
	// 첫 호출에는 연속조회 헤더를 아예 보내지 않는다 — 빈 값을 보내면 서버가 이어받기로
	// 오해할 여지가 있다.
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
