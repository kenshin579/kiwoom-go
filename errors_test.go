package kiwoom_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

func TestAPIError(t *testing.T) {
	err := &kiwoom.APIError{StatusCode: 200, ReturnCode: 1505, ReturnMsg: "해당 API ID는 존재하지 않습니다", APIID: "ka99999"}

	if !kiwoom.IsCode(err, 1505) {
		t.Error("IsCode(1505) = false")
	}
	if kiwoom.IsCode(err, 1501) {
		t.Error("IsCode(1501) = true, want false")
	}
	if kiwoom.IsCode(errors.New("boom"), 1505) {
		t.Error("APIError 가 아니면 false 여야 한다")
	}

	wrapped := fmt.Errorf("조회 실패: %w", err)
	if !kiwoom.IsCode(wrapped, 1505) {
		t.Error("감싼 에러에서도 찾아야 한다")
	}
	if err.Error() == "" {
		t.Error("Error() 가 비어 있다")
	}
}

func TestAPIError_HTTPOnly(t *testing.T) {
	// 본문을 못 읽은 전송 실패 — ReturnCode 가 0 이어도 에러다
	err := &kiwoom.APIError{StatusCode: 500, ReturnMsg: "Internal Server Error", APIID: "ka10004"}
	if kiwoom.IsCode(err, 0) {
		t.Error("ReturnCode 0 을 코드로 취급하면 안 된다")
	}
	if err.Error() == "" {
		t.Error("Error() 가 비어 있다")
	}
}
