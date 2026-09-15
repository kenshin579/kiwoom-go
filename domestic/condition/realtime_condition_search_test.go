package condition

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

	"github.com/kenshin579/kiwoom-go/internal/wstransport"
)

// condRealtimeFake 는 ka10173 을 흉내내는 서버다.
//
// 조건검색은 337개 중 유일하게 **응답이 두 벌**이라 요청 하나에 답을 그대로 돌려주는
// 기존 newFakeWS 로는 부족하다. 여기서는 CNSRREQ 를 받으면 조회 응답과 REAL 푸시를
// 둘 다 보낸다.
type condRealtimeFake struct {
	// pushFirst 면 조회 응답보다 **푸시를 먼저** 보낸다.
	//
	// 실제로 그럴 수 있다 — 요청하는 순간 서버가 밀기 시작하므로, 응답을 받은 뒤에
	// 라우팅을 붙이는 구현이라면 이 첫 건이 소리 없이 사라진다.
	pushFirst bool
	// returnCode 가 0 이 아니면 업무 오류로 답하고 푸시는 보내지 않는다.
	returnCode int
}

func (s condRealtimeFake) start(t *testing.T) string {
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
			if m["trnm"] != "CNSRREQ" {
				continue
			}
			if s.returnCode != 0 {
				_ = write(map[string]any{
					"trnm": "CNSRREQ", "return_code": s.returnCode, "return_msg": "조건검색식이 없습니다",
				})
				continue
			}
			resp := map[string]any{
				"trnm": "CNSRREQ", "seq": "4", "return_code": 0, "return_msg": "",
				"data": []any{map[string]any{"jmcode": "A005930"}},
			}
			push := map[string]any{
				"trnm": "REAL",
				"data": []any{map[string]any{
					"type": "02", "name": "조건검색", "item": "005930",
					"values": map[string]any{
						"841": "4", "9001": "005930", "843": "I", "20": "152028",
						"907": "2", "9999": "새필드",
					},
				}},
			}
			if s.pushFirst {
				_ = write(push)
				_ = write(resp)
				continue
			}
			_ = write(resp)
			_ = write(push)
		}
	}))
	t.Cleanup(srv.Close)
	return strings.Replace(srv.URL, "http://", "ws://", 1)
}

func newRealtimeConditionClient(t *testing.T, s condRealtimeFake) *Client {
	t.Helper()
	return New(wstransport.New(s.start(t), "", condStubToken{}))
}

type condStubToken struct{}

func (condStubToken) Token(context.Context) (string, error) { return "TKN", nil }
func (condStubToken) Invalidate()                           {}

// 이 Task 의 핵심이다. 타입만 맞춰 놓고 푸시가 오지 않으면 뜻이 없다 —
// 조회 응답과 그 뒤의 REAL 푸시를 끝에서 끝까지 본다.
//
// Connect 를 부르지 않는다. 첫 호출이 스스로 붙어야 한다(게으른 연결).
func TestRequestDomesticRealtimeConditionSearch_조회응답과_푸시를_모두_받는다(t *testing.T) {
	for _, pushFirst := range []bool{false, true} {
		name := "응답_먼저"
		if pushFirst {
			name = "푸시_먼저"
		}
		t.Run(name, func(t *testing.T) {
			c := newRealtimeConditionClient(t, condRealtimeFake{pushFirst: pushFirst})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, ch, err := c.RequestDomesticRealtimeConditionSearch(ctx,
				RequestDomesticRealtimeConditionSearchRequest{Seq: "4", SearchType: "1", StexTp: "K"})
			if err != nil {
				t.Fatalf("RequestDomesticRealtimeConditionSearch: %v", err)
			}
			if resp.Trnm != "CNSRREQ" || resp.Seq != "4" {
				t.Errorf("응답 = %+v, 기대 CNSRREQ/4", resp)
			}
			if len(resp.Data) != 1 || resp.Data[0].Jmcode != "A005930" {
				t.Errorf("Data = %+v, 기대 jmcode A005930 한 건", resp.Data)
			}

			select {
			case ev := <-ch:
				if ev.Err != nil {
					t.Fatalf("Err = %v", ev.Err)
				}
				if ev.Symbol != "005930" || ev.Name != "조건검색" {
					t.Errorf("Symbol/Name = %s/%s, 기대 005930/조건검색", ev.Symbol, ev.Name)
				}
				want := DomesticRealtimeConditionMatch{
					SequenceNumber: "4", StockOrSectorCode: "005930",
					InsertDeleteType: "I", TradeTime: "152028", TradeSide: "2",
				}
				if ev.Value != want {
					t.Errorf("Value = %+v, 기대 %+v", ev.Value, want)
				}
				if ev.Raw["9999"] != "새필드" {
					t.Errorf("Raw[9999] = %q, 기대 새필드 — 모르는 FID 가 버려졌다", ev.Raw["9999"])
				}
			case <-time.After(5 * time.Second):
				t.Fatal("편입 푸시가 채널로 오지 않았다")
			}
		})
	}
}

// 업무 오류면 채널을 주지 않는다. 주면 사용자는 영영 오지 않을 푸시를 기다린다.
func TestRequestDomesticRealtimeConditionSearch_업무오류는_APIError_다(t *testing.T) {
	c := newRealtimeConditionClient(t, condRealtimeFake{returnCode: 8005})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, ch, err := c.RequestDomesticRealtimeConditionSearch(ctx,
		RequestDomesticRealtimeConditionSearchRequest{Seq: "4", SearchType: "1", StexTp: "K"})

	var ae *wstransport.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, 기대 *APIError", err)
	}
	if ae.ReturnCode != 8005 || ae.Trnm != "CNSRREQ" {
		t.Errorf("APIError = %+v, 기대 CNSRREQ/8005", ae)
	}
	if resp != nil || ch != nil {
		t.Error("에러인데 응답이나 채널을 줬다")
	}
}

// ctx 취소가 채널을 닫는지 본다. 닫히지 않으면 소비자는 for range 에서 영원히 막힌다.
func TestRequestDomesticRealtimeConditionSearch_ctx_취소가_채널을_닫는다(t *testing.T) {
	c := newRealtimeConditionClient(t, condRealtimeFake{})
	ctx, cancel := context.WithCancel(context.Background())

	_, ch, err := c.RequestDomesticRealtimeConditionSearch(ctx,
		RequestDomesticRealtimeConditionSearchRequest{Seq: "4", SearchType: "1", StexTp: "K"})
	if err != nil {
		t.Fatalf("RequestDomesticRealtimeConditionSearch: %v", err)
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch { // 남은 이벤트를 비우면 닫힌다
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("채널이 닫히지 않았다")
	}
}

// 일련번호가 없으면 요청을 보내기도 전에 거절한다. 라우팅 열쇠가 없어 푸시가 한 건도
// 오지 않는데, 그 침묵은 "조건에 걸린 종목이 없다" 와 구분되지 않는다.
func TestRequestDomesticRealtimeConditionSearch_빈_일련번호를_거절한다(t *testing.T) {
	c := newRealtimeConditionClient(t, condRealtimeFake{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, _, err := c.RequestDomesticRealtimeConditionSearch(ctx,
		RequestDomesticRealtimeConditionSearchRequest{SearchType: "1", StexTp: "K"}); err == nil {
		t.Fatal("빈 seq 를 받아들였다")
	}
}

// 봉투는 그대로 흘러야 한다. 구멍·느린 소비자 알림에 값을 채우면 사용자가 빈 값을
// 진짜 편입으로 읽는다.
func TestDecodeDomesticRealtimeConditionMatch_에러는_값을_비운다(t *testing.T) {
	want := &wstransport.SlowConsumerError{Type: "조건검색", Item: "4", Dropped: 3}
	ev := decodeDomesticRealtimeConditionMatch(wstransport.Delivery{Err: want})
	if !errors.Is(ev.Err, error(want)) {
		t.Errorf("Err = %v, 기대 그대로 전달", ev.Err)
	}
	if ev.Value != (DomesticRealtimeConditionMatch{}) {
		t.Errorf("Value = %+v, 기대 영값", ev.Value)
	}
	if ev.Raw != nil {
		t.Errorf("Raw = %v, 기대 nil", ev.Raw)
	}
}

// 응답 구조체에 봉투 필드가 남아 있으면 안 된다. 전송 계층이 이미 가른 판단을 공개
// 표면에 한 번 더 두면, 그것을 보고 성공·실패를 정하려는 사람이 생긴다.
func TestRealtimeConditionResponse_봉투_필드가_없다(t *testing.T) {
	b, err := json.Marshal(RequestDomesticRealtimeConditionSearchResponse{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{"return_code", "return_msg"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("응답에 %s 가 남아 있다: %s", banned, b)
		}
	}
	// 요청 쪽도 같다 — trnm 은 전송 직전에 끼운다.
	rb, err := json.Marshal(RequestDomesticRealtimeConditionSearchRequest{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(rb), "trnm") {
		t.Errorf("요청에 trnm 이 남아 있다: %s", rb)
	}
}
