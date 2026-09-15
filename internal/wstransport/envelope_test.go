package wstransport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// 서버는 return_code 를 엔드포인트에 따라 int 로도 숫자 문자열로도 보낸다.
// 봉투가 int 만 가정하면 문자열이 오는 순간 메시지 전체를 읽지 못하고, 로그인 거부가
// "해석 못 한 프레임" 으로 사라진다 — 붙은 줄 알았는데 아무 일도 일어나지 않는다.
func TestEnvelope_로그인_return_code_는_두_형태를_다_받는다(t *testing.T) {
	tests := []struct {
		name    string
		code    any
		wantErr bool
	}{
		{"int 0", 0, false},
		{"문자열 0", "0", false},
		{"앞자리 0 이 붙은 문자열", "0000", false},
		{"int 업무오류", 8005, true},
		{"문자열 업무오류", "8005", true},
		{"빈 문자열", "", false},
		{"필드 없음", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			f.loginCodeRaw = tt.code

			c := New(f.wsURL(), "", &stubToken{token: "TKN"})
			defer func() { _ = c.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := c.Connect(ctx)
			if tt.wantErr {
				var le *LoginError
				if !errors.As(err, &le) {
					t.Fatalf("code=%v → %v, 기대 *LoginError", tt.code, err)
				}
				if le.ReturnCode != 8005 {
					t.Errorf("ReturnCode = %d, 기대 8005", le.ReturnCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("code=%v → %v, 기대 성공", tt.code, err)
			}
		})
	}
}

// 요청 응답 쪽도 같다. 문자열 코드가 오면 *APIError 가 나와야 하고,
// "0000" 은 정상으로 통과해야 한다.
func TestRequest_return_code_는_두_형태를_다_받는다(t *testing.T) {
	tests := []struct {
		name     string
		code     any
		wantCode int // 0 이면 성공 기대
	}{
		{"int 0", 0, 0},
		{"문자열 0", "0", 0},
		{"앞자리 0 이 붙은 문자열", "0000", 0},
		{"필드 없음", nil, 0},
		{"int 업무오류", 8005, 8005},
		{"문자열 업무오류", "8005", 8005},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeServer(t)
			f.onConn = func(t *testing.T, n int, ws *websocket.Conn) {
				go func() {
					time.Sleep(30 * time.Millisecond)
					msg := map[string]any{"trnm": "CNSRLST", "return_msg": "메시지"}
					if tt.code != nil {
						msg["return_code"] = tt.code
					}
					_ = f.writeJSON(ws, msg)
				}()
			}

			c := New(f.wsURL(), "", &stubToken{token: "TKN"})
			defer func() { _ = c.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := c.Request(ctx, "CNSRLST", map[string]any{"trnm": "CNSRLST"}, nil)
			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("code=%v → %v, 기대 성공", tt.code, err)
				}
				return
			}
			var ae *APIError
			if !errors.As(err, &ae) {
				t.Fatalf("code=%v → %v, 기대 *APIError", tt.code, err)
			}
			if ae.ReturnCode != tt.wantCode {
				t.Errorf("ReturnCode = %d, 기대 %d", ae.ReturnCode, tt.wantCode)
			}
		})
	}
}
