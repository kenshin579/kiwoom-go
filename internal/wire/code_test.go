package wire_test

import (
	"encoding/json"
	"testing"

	"github.com/kenshin579/kiwoom-go/internal/wire"
)

// 서버가 같은 값을 int 로도 숫자 문자열로도 보낸다. 두 형태를 한 표로 못박는다 —
// 한쪽만 통과하는 구현이 다시 들어오면 여기가 잡는다.
func TestCode_int와_숫자문자열을_둘_다_받는다(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"int 0", `{"return_code":0}`, 0},
		{"문자열 0", `{"return_code":"0"}`, 0},
		{"앞자리 0 이 붙은 문자열", `{"return_code":"0000"}`, 0},
		{"int 업무오류", `{"return_code":8005}`, 8005},
		{"문자열 업무오류", `{"return_code":"8005"}`, 8005},
		{"앞자리 0 이 붙은 업무오류", `{"return_code":"08005"}`, 8005},
		{"필드 없음", `{}`, 0},
		{"빈 문자열", `{"return_code":""}`, 0},
		{"null", `{"return_code":null}`, 0},
		{"공백이 섞인 문자열", `{"return_code":" 8005 "}`, 8005},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var env struct {
				ReturnCode wire.Code `json:"return_code"`
			}
			if err := json.Unmarshal([]byte(tt.body), &env); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tt.body, err)
			}
			if env.ReturnCode.Int() != tt.want {
				t.Errorf("%s → %d, 기대 %d", tt.body, env.ReturnCode.Int(), tt.want)
			}
		})
	}
}

// 숫자로 읽을 수 없는 값을 0(정상)으로 떨어뜨리면 이 타입을 만든 이유가 사라진다.
func TestCode_숫자가_아니면_에러다(t *testing.T) {
	for _, body := range []string{
		`{"return_code":"오류"}`,
		`{"return_code":true}`,
		`{"return_code":{"a":1}}`,
		`{"return_code":[0]}`,
	} {
		var env struct {
			ReturnCode wire.Code `json:"return_code"`
		}
		if err := json.Unmarshal([]byte(body), &env); err == nil {
			t.Errorf("%s 가 통과했다 (code=%d) — 못 읽은 것을 정상으로 읽으면 안 된다", body, env.ReturnCode)
		}
	}
}
