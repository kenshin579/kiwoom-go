package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kenshin579/kiwoom-go/internal/transport"
)

type stubToken struct {
	val         string
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
		Body: map[string]string{"stk_cd": "005930"},
	}, &out, transport.WithCont(transport.Meta{ContYN: "Y", NextKey: "K1"}))
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

func TestDo_첫호출은_연속조회헤더가_없다(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Cont-Yn"]; ok {
			t.Error("cont-yn 헤더가 없어야 한다")
		}
		if _, ok := r.Header["Next-Key"]; ok {
			t.Error("next-key 헤더가 없어야 한다")
		}
		_, _ = w.Write([]byte(`{"return_code":0}`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	if _, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestDo_본문오류는_200이어도_에러다(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"return_code":1505,"return_msg":"해당 API ID는 존재하지 않습니다"}`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	_, err := c.Do(context.Background(), transport.Request{APIID: "ka99999", Path: "/x"}, nil)
	if err == nil {
		t.Fatal("에러여야 한다 — 키움은 업무 오류를 HTTP 200 으로 보낸다")
	}
	var ae *transport.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("APIError 여야 한다: %v", err)
	}
	if ae.ReturnCode != 1505 || ae.APIID != "ka99999" {
		t.Errorf("APIError = %+v", ae)
	}
}

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

func TestDo_500은_재시도하지_않는다(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`서버 오류`))
	}))
	defer srv.Close()

	c := transport.New(srv.URL, srv.Client(), &stubToken{val: "TK"})
	_, err := c.Do(context.Background(), transport.Request{APIID: "ka10004", Path: "/x"}, nil)
	if err == nil {
		t.Fatal("에러여야 한다")
	}
	if hits != 1 {
		t.Errorf("호출 = %d, want 1 (5xx 재시도 없음 — 키움 정책을 모른다)", hits)
	}
	var ae *transport.APIError
	if !errors.As(err, &ae) || ae.StatusCode != 500 || ae.Body == "" {
		t.Errorf("APIError 에 상태코드와 원문이 있어야 한다: %+v", ae)
	}
}
