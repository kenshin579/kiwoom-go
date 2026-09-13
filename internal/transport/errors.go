// Package transport 는 키움 REST 호출을 담당한다.
package transport

import "fmt"

// APIError 는 키움 API 호출 실패다.
//
// 두 층을 하나로 모은다. HTTP 자체가 실패한 경우(StatusCode != 200)와, HTTP 200 으로 왔지만
// 본문의 return_code 가 0 이 아닌 경우다. **키움은 업무 오류를 200 + return_code 로 돌려주므로**
// 상태코드만 보면 실패를 놓친다.
type APIError struct {
	StatusCode int    // HTTP 상태코드
	ReturnCode int    // 본문 return_code. 전송 실패라 본문을 못 읽었으면 0
	ReturnMsg  string // 본문 return_msg 또는 HTTP 상태 문구
	APIID      string // 호출한 api-id(어느 API 가 실패했는지)
	Body       string // 파싱 실패 시 원문 일부
}

func (e *APIError) Error() string {
	if e.ReturnCode != 0 {
		return fmt.Sprintf("kiwoom: %s 실패 (return_code=%d): %s", e.APIID, e.ReturnCode, e.ReturnMsg)
	}
	return fmt.Sprintf("kiwoom: %s 실패 (HTTP %d): %s", e.APIID, e.StatusCode, e.ReturnMsg)
}
