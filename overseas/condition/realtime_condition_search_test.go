package condition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

type usCondStubToken struct{}

func (usCondStubToken) Token(context.Context) (string, error) { return "TKN", nil }
func (usCondStubToken) Invalidate()                           {}

// usCondFakeWS 는 usa20290 을 흉내낸다. GCNSRREQ 를 받으면 조회 응답과 REAL 푸시를
// 둘 다 보낸다. 본문은 스펙의 응답 예제 그대로다 — 조회 data 는 stex_tp, 푸시 원소는
// stexTp 로 철자가 갈린다.
func usCondFakeWS(t *testing.T) string {
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
		read := func() (map[string]any, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, b, err := c.Read(ctx)
			if err != nil {
				return nil, err
			}
			var m map[string]any
			return m, json.Unmarshal(b, &m)
		}

		if _, err := read(); err != nil { // LOGIN
			return
		}
		if err := write(map[string]any{"trnm": "LOGIN", "return_code": 0}); err != nil {
			return
		}
		for {
			m, err := read()
			if err != nil {
				return
			}
			if m["trnm"] != "GCNSRREQ" {
				continue
			}
			_ = write(map[string]any{
				"trnm": "GCNSRREQ", "seq": "2", "return_code": 0,
				"data": []any{map[string]any{"stex_tp": "ND", "jmcode": "AAPL"}},
			})
			_ = write(map[string]any{
				"trnm": "REAL",
				"data": []any{map[string]any{
					"type": "S2", "name": "조건검색", "item": "COIN", "stexTp": "ND",
					"values": map[string]any{
						"20": "230156", "841": "2", "843": "I", "907": "2", "9001": "COIN",
					},
				}},
			})
		}
	}))
	t.Cleanup(srv.Close)
	return strings.Replace(srv.URL, "http://", "ws://", 1)
}

// 조회 응답은 타입이 붙고 푸시는 Raw 맵으로만 온다 — 이 API 하나만 그렇다.
func TestRequestOverseasRealtimeConditionSearch_조회응답과_푸시를_모두_받는다(t *testing.T) {
	c := New(wstransport.New(usCondFakeWS(t), "", usCondStubToken{}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, ch, err := c.RequestOverseasRealtimeConditionSearch(ctx,
		RequestOverseasRealtimeConditionSearchRequest{Seq: "2", SearchType: "1"})
	if err != nil {
		t.Fatalf("RequestOverseasRealtimeConditionSearch: %v", err)
	}
	if resp.Trnm != "GCNSRREQ" || resp.Seq != "2" {
		t.Errorf("응답 = %+v, 기대 GCNSRREQ/2", resp)
	}
	if len(resp.Data) != 1 || resp.Data[0].Jmcode != "AAPL" || resp.Data[0].StexTp != "ND" {
		t.Errorf("Data = %+v, 기대 AAPL/ND 한 건", resp.Data)
	}

	select {
	case ev := <-ch:
		if ev.Err != nil {
			t.Fatalf("Err = %v", ev.Err)
		}
		if ev.Symbol != "COIN" {
			t.Errorf("Symbol = %q, 기대 COIN", ev.Symbol)
		}
		// 값 구조체는 늘 비어 있다. 이름표가 없어서지 푸시가 없어서가 아니다.
		if ev.Value != (OverseasRealtimeConditionMatch{}) {
			t.Errorf("Value = %+v, 기대 영값", ev.Value)
		}
		// 받은 FID 는 Raw 에 전부 있어야 한다. 여기가 비면 이 API 는 쓸모가 없다.
		for fid, want := range map[string]string{
			"841": "2", "9001": "COIN", "843": "I", "20": "230156", "907": "2",
		} {
			if ev.Raw[fid] != want {
				t.Errorf("Raw[%s] = %q, 기대 %q", fid, ev.Raw[fid], want)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("편입 푸시가 채널로 오지 않았다")
	}
}

// 거래소구분은 문서가 스스로 어긋난 자리다 — 표는 stexTp, 예제는 stex_tp.
// 하나만 받으면 반대쪽이 올 때 조용히 빈다.
func TestOverseasRealtimeConditionDataItem_거래소구분을_두_철자로_받는다(t *testing.T) {
	cases := map[string]string{
		"스펙 표 철자": `{"jmcode":"AAPL","stexTp":"ND"}`,
		"예제 철자":   `{"jmcode":"AAPL","stex_tp":"ND"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var it RequestOverseasRealtimeConditionSearchDataItem
			if err := json.Unmarshal([]byte(body), &it); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if it.Jmcode != "AAPL" || it.StexTp != "ND" {
				t.Errorf("= %+v, 기대 AAPL/ND", it)
			}
		})
	}
}

func TestRequestOverseasRealtimeConditionSearch_ctx_취소가_채널을_닫는다(t *testing.T) {
	c := New(wstransport.New(usCondFakeWS(t), "", usCondStubToken{}))
	ctx, cancel := context.WithCancel(context.Background())

	_, ch, err := c.RequestOverseasRealtimeConditionSearch(ctx,
		RequestOverseasRealtimeConditionSearchRequest{Seq: "2", SearchType: "1"})
	if err != nil {
		t.Fatalf("RequestOverseasRealtimeConditionSearch: %v", err)
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}
}

// 봉투 필드는 공개 표면에 없어야 한다(국내 짝과 같은 규칙).
func TestOverseasRealtimeConditionResponse_봉투_필드가_없다(t *testing.T) {
	b, err := json.Marshal(RequestOverseasRealtimeConditionSearchResponse{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{"return_code", "return_msg"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("응답에 %s 가 남아 있다: %s", banned, b)
		}
	}
	rb, err := json.Marshal(RequestOverseasRealtimeConditionSearchRequest{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(rb), "trnm") {
		t.Errorf("요청에 trnm 이 남아 있다: %s", rb)
	}
}
