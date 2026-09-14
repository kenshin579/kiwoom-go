// Package wire 는 REST 와 WebSocket 이 **함께** 쓰는 응답 봉투 조각을 담는다.
//
// 두 전송이 같은 서버의 같은 규약을 읽는데 각자 타입을 두면, 한쪽만 고친 날 다른 쪽이
// 조용히 어긋난다. 실제로 그런 적이 있다 — REST 는 return_code 를 int 로만, WebSocket 은
// 역시 int 로만 가정했고, 서버가 문자열로 보내는 엔드포인트에서 둘 다 틀렸다.
package wire

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Code 는 키움 응답의 return_code 다. **int 와 숫자 문자열을 둘 다 받는다.**
//
// 공식 파이썬 클라이언트 kiwoom/core/errors.py 의 normalize_return_code 가 적어 둔 그대로다:
// 서버는 같은 값을 엔드포인트에 따라 int(`0`, `8005`) 로도 숫자 문자열(`"0"`, `"0000"`,
// `"8005"`) 로도 보낸다. 앞자리 0 이 붙은 `"0000"` 도 0 이다.
//
// 한쪽만 가정하면 어떻게 되는지가 이 타입이 있는 이유다. int 로만 선언한 구조체에
// `{"return_code":"8005"}` 를 넣으면 json.Unmarshal 이 **봉투 전체**를 실패시키고,
// 그 에러를 버리는 코드에서는 봉투가 영값으로 남아 return_code == 0 — 업무 오류가
// 조용히 성공이 된다.
//
// 필드가 아예 없거나 null, 또는 빈 문자열이면 "코드 없음" 으로 0 이다. 그 밖에 숫자로
// 읽을 수 없는 값은 **에러**다 — 못 읽은 것을 0(정상)으로 떨어뜨리면 같은 구멍이 다시 난다.
type Code int

// Int 는 코드를 int 로 준다. 공개 에러 타입이 int 필드를 쓰므로 변환 자리를 하나로 모은다.
func (c Code) Int() int { return int(c) }

// UnmarshalJSON 은 int 와 숫자 문자열을 둘 다 받는다.
func (c *Code) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*c = 0
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		str = strings.TrimSpace(str)
		if str == "" {
			*c = 0
			return nil
		}
		// Atoi 가 앞자리 0 ("0000")과 부호를 그대로 받는다.
		n, err := strconv.Atoi(str)
		if err != nil {
			return fmt.Errorf("kiwoom: return_code 를 숫자로 읽지 못했다: %q", str)
		}
		*c = Code(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("kiwoom: return_code 형식을 알 수 없다: %s", s)
	}
	*c = Code(n)
	return nil
}
