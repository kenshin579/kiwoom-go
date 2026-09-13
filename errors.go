package kiwoom

import (
	"errors"

	"github.com/kenshin579/kiwoom-go/internal/transport"
)

// APIError 는 키움 API 호출 실패다. errors.As 로 아래 필드에 접근한다.
//
//	StatusCode int    // HTTP 상태코드
//	ReturnCode int    // 본문 return_code. 전송 실패라 본문을 못 읽었으면 0
//	ReturnMsg  string // 본문 return_msg 또는 HTTP 상태 문구
//	APIID      string // 호출한 api-id
//	Body       string // 파싱 실패 시 원문 일부
//
// 키움은 업무 오류를 **HTTP 200 + return_code** 로 돌려준다. 상태코드만 보면 실패를 놓친다.
type APIError = transport.APIError

// IsCode 는 err 가 주어진 키움 return_code 의 *APIError 인지 판별한다.
// code 0 은 "오류 없음" 이므로 항상 false 다.
func IsCode(err error, code int) bool {
	if code == 0 {
		return false
	}
	var ae *APIError
	return errors.As(err, &ae) && ae.ReturnCode == code
}
