package kiwoom

import "github.com/kenshin579/kiwoom-go/internal/transport"

// Meta 는 응답 헤더에서 읽은 연속조회 정보다.
//
// ContYN 이 "Y" 면 NextKey 를 다음 호출에 넣어 이어서 조회한다. 생성된 모든 API 메서드가
// 이 값을 함께 돌려준다.
type Meta = transport.Meta

// CallOption 은 호출 한 건에 붙이는 옵션이다.
//
// 클라이언트를 만들 때 쓰는 Option 과 다르다 — 이쪽은 개별 API 호출에 붙는다.
type CallOption = transport.Option

// WithCont 는 연속조회를 이어간다. 직전 응답의 Meta 를 그대로 넘긴다.
//
//	resp, meta, err := c.DomesticQuote.GetDomesticStockQuote(ctx, req)
//	for meta.ContYN == "Y" {
//	    resp, meta, err = c.DomesticQuote.GetDomesticStockQuote(ctx, req, kiwoom.WithCont(meta))
//	}
//
// 자동 반복을 넣지 않은 이유: 어떤 API 가 몇 페이지까지 가는지 모르는 채로 자동화하면
// 사용자가 모르는 사이에 호출 한도를 태운다.
func WithCont(m Meta) CallOption { return transport.WithCont(m) }
