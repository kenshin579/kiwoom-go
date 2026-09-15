package condition_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kenshin579/kiwoom-go/domestic/condition"
	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

// 조건검색 응답이 실제로 파싱되는지 끝에서 끝까지 본다.
//
// 단위 테스트로는 안 잡히던 것이 둘 있었다.
//
// 하나, 아무도 Connect 를 부르지 않아 생성된 메서드가 전부 errNotConnected 로 떨어졌다.
// 이 테스트는 Connect 를 부르지 않는다 — 첫 호출이 스스로 붙어야 한다.
//
// 둘, 생성물의 `ReturnCode string` 이 서버가 보내는 int 를 못 받아 응답 파싱이 통째로
// 실패했다. 지금은 그 필드가 없다(봉투 판단은 전송 계층이 한다).
func TestListDomesticConditionSearches_서버_응답을_파싱한다(t *testing.T) {
	srv := newFakeWS(t, map[string]any{
		"trnm":        "CNSRLST",
		"return_code": 0, // **int** 다. 스펙은 String 이라 적었다
		"return_msg":  "",
		"data": []any{
			map[string]any{"seq": "0", "name": "내조건"},
			map[string]any{"seq": "1", "name": "급등주"},
		},
	})

	c := condition.New(wstransport.New(srv, "", stubToken{}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect 를 부르지 않는다. 그것이 요점이다.
	resp, err := c.ListDomesticConditionSearches(ctx, condition.ListDomesticConditionSearchesRequest{})
	if err != nil {
		t.Fatalf("ListDomesticConditionSearches: %v", err)
	}
	if resp.Trnm != "CNSRLST" {
		t.Errorf("Trnm = %q, 기대 CNSRLST", resp.Trnm)
	}
	if len(resp.Data) != 2 || resp.Data[0].Name != "내조건" || resp.Data[1].Seq != "1" {
		t.Errorf("Data = %+v, 기대 2건", resp.Data)
	}
}

// 업무 오류는 *wstransport.APIError(= kiwoom.WSAPIError)로 와야 한다.
// 응답 구조체의 return_code 를 보고 사용자가 직접 가르는 것이 아니다.
func TestListDomesticConditionSearches_업무오류는_APIError_다(t *testing.T) {
	for _, code := range []any{8005, "8005"} {
		srv := newFakeWS(t, map[string]any{
			"trnm": "CNSRLST", "return_code": code, "return_msg": "조건검색식이 없습니다",
		})

		c := condition.New(wstransport.New(srv, "", stubToken{}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		resp, err := c.ListDomesticConditionSearches(ctx, condition.ListDomesticConditionSearchesRequest{})
		cancel()

		var ae *wstransport.APIError
		if !errors.As(err, &ae) {
			t.Fatalf("return_code=%v → %v, 기대 *APIError", code, err)
		}
		if ae.ReturnCode != 8005 || ae.Trnm != "CNSRLST" {
			t.Errorf("APIError = %+v, 기대 CNSRLST/8005", ae)
		}
		if resp != nil {
			t.Error("에러인데 응답을 줬다")
		}
	}
}

// 응답 구조체에 return_code·return_msg 가 없어야 한다. 전송 계층이 이미 가른 판단을
// 공개 표면에 한 번 더 두면, 그것을 보고 성공·실패를 정하려는 사람이 생긴다.
func TestConditionResponse_봉투_필드가_없다(t *testing.T) {
	var resp condition.ListDomesticConditionSearchesResponse
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{"return_code", "return_msg"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("응답에 %s 가 남아 있다: %s", banned, b)
		}
	}
}

type stubToken struct{}

func (stubToken) Token(context.Context) (string, error) { return "TKN", nil }
func (stubToken) Invalidate()                           {}

// newFakeWS 는 로그인에 답하고, 그 뒤 받는 요청마다 reply 를 그대로 돌려주는 서버다.
// ws:// URL 을 준다.
func newFakeWS(t *testing.T, reply map[string]any) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		write := func(v any) error {
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return c.Write(ctx, websocket.MessageText, b)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err = c.Read(ctx) // LOGIN
		cancel()
		if err != nil {
			return
		}
		if err := write(map[string]any{"trnm": "LOGIN", "return_code": 0}); err != nil {
			return
		}
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _, err := c.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			if err := write(reply); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return strings.Replace(srv.URL, "http://", "ws://", 1)
}
