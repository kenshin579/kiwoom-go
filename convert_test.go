package kiwoom_test

import (
	"testing"
	"time"

	"github.com/kenshin579/kiwoom-go"
)

func TestDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1234", "1234", true},
		{"+1234", "1234", true}, // 키움은 부호를 붙여 보낸다
		{"-56", "-56", true},
		{"0012", "12", true}, // 앞자리 0
		{"1,234,567", "1234567", true},
		{"  789  ", "789", true},
		{"12.34", "12.34", true},
		{"", "0", false}, // 값 없음 — 0 과 구분된다
		{"abc", "0", false},
		{"--1", "0", false},
	}
	for _, c := range cases {
		got, ok := kiwoom.DecimalOK(c.in)
		if ok != c.ok {
			t.Errorf("DecimalOK(%q) ok=%v, want %v", c.in, ok, c.ok)
		}
		if got.String() != c.want {
			t.Errorf("DecimalOK(%q) = %s, want %s", c.in, got.String(), c.want)
		}
		if kiwoom.Decimal(c.in).String() != c.want {
			t.Errorf("Decimal(%q) = %s, want %s", c.in, kiwoom.Decimal(c.in).String(), c.want)
		}
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1234", 1234, true},
		{"+1234", 1234, true},
		{"-56", -56, true},
		{"0012", 12, true},
		{"1,234", 1234, true},
		{"", 0, false},
		{"12.34", 0, false},                 // 정수가 아니다
		{"999999999999999999999", 0, false}, // int64 초과 — 조용히 잘리면 안 된다
		{"-999999999999999999999", 0, false},
	}
	for _, c := range cases {
		got, ok := kiwoom.IntOK(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("IntOK(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestDate(t *testing.T) {
	kst := time.FixedZone("KST", 9*60*60)
	got, ok := kiwoom.DateOK("20260913")
	if !ok {
		t.Fatal("DateOK(20260913) ok=false")
	}
	want := time.Date(2026, 9, 13, 0, 0, 0, 0, kst)
	if !got.Equal(want) {
		t.Errorf("DateOK = %v, want %v", got, want)
	}
	for _, bad := range []string{"", "2026091", "20261301", "abcdefgh"} {
		if _, ok := kiwoom.DateOK(bad); ok {
			t.Errorf("DateOK(%q) ok=true, want false", bad)
		}
	}
}

func TestDateTime(t *testing.T) {
	kst := time.FixedZone("KST", 9*60*60)
	got, ok := kiwoom.DateTimeOK("20241107083713")
	if !ok {
		t.Fatal("DateTimeOK ok=false")
	}
	want := time.Date(2024, 11, 7, 8, 37, 13, 0, kst)
	if !got.Equal(want) {
		t.Errorf("DateTimeOK = %v, want %v", got, want)
	}
	if _, ok := kiwoom.DateTimeOK("20241107"); ok {
		t.Error("길이가 다르면 false 여야 한다")
	}
}
