// Package kiwoom 은 키움증권 REST API 의 Go 클라이언트다.
package kiwoom

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// KST 는 키움이 쓰는 시간대. 날짜·시각 문자열은 전부 이 기준으로 해석한다.
var KST = time.FixedZone("KST", 9*60*60)

// clean 은 키움 숫자 문자열의 껍데기를 벗긴다.
//
// 키움은 부호를 붙여(`"+1234"`) 보내고, 자리를 채우려 앞에 0 을 두며(`"0012"`),
// 표시용 콤마가 섞여 오는 필드도 있다. 이 셋을 한 곳에서 처리해 사용자가 각자 밟지 않게 한다.
//
// 콤마를 무조건 지우는 것은 확인된 판단이다 — 공식 스펙의 응답 예시를 전수 조사한 결과
// 숫자 안에 콤마가 들어간 사례가 없고 소수점은 언제나 `.` 다. 유럽식 소수 구분자로
// 해석할 여지가 없으므로 천 단위 구분자로만 다룬다.
func clean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	return strings.TrimPrefix(s, "+")
}

// DecimalOK 는 금액·수량 문자열을 decimal 로 바꾼다.
// 빈 문자열은 (0, false) — "값 없음" 과 "0" 을 구분하기 위해서다.
func DecimalOK(s string) (decimal.Decimal, bool) {
	c := clean(s)
	if c == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(c)
	if err != nil {
		return decimal.Zero, false
	}
	return d, true
}

// Decimal 은 DecimalOK 의 값만 쓰는 축약형. 실패하면 0 이다.
func Decimal(s string) decimal.Decimal { d, _ := DecimalOK(s); return d }

// IntOK 는 정수 문자열을 int64 로 바꾼다. 소수점이 있으면 실패다.
func IntOK(s string) (int64, bool) {
	c := clean(s)
	if c == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(c, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Int 는 IntOK 의 값만 쓰는 축약형. 실패하면 0 이다.
func Int(s string) int64 { n, _ := IntOK(s); return n }

// DateOK 는 YYYYMMDD 를 KST 자정으로 바꾼다.
func DateOK(s string) (time.Time, bool) { return parseTime(s, "20060102") }

// Date 는 DateOK 의 값만 쓰는 축약형. 실패하면 제로 time.Time 이다.
func Date(s string) time.Time { t, _ := DateOK(s); return t }

// DateTimeOK 는 YYYYMMDDHHMMSS 를 KST 시각으로 바꾼다.
func DateTimeOK(s string) (time.Time, bool) { return parseTime(s, "20060102150405") }

// DateTime 은 DateTimeOK 의 값만 쓰는 축약형. 실패하면 제로 time.Time 이다.
func DateTime(s string) time.Time { t, _ := DateTimeOK(s); return t }

// parseTime 은 길이를 먼저 확인한다 — 길이가 다른 값을 부분 일치로 통과시키는 함정이 있다.
func parseTime(s, layout string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) != len(layout) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(layout, s, KST)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
