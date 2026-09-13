package kiwoom_test

import (
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

func TestNewClient_기본은_운영도메인(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.BaseURL(); got != "https://api.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClient_모의투자(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithMock())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.BaseURL(); got != "https://mockapi.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClient_키가_없으면_에러(t *testing.T) {
	if _, err := kiwoom.NewClient("", "SK"); err == nil {
		t.Error("appKey 가 비면 에러여야 한다")
	}
	if _, err := kiwoom.NewClient("AK", ""); err == nil {
		t.Error("secretKey 가 비면 에러여야 한다")
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	t.Setenv("KIWOOM_ENV", "mock")

	c, err := kiwoom.NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	if got := c.BaseURL(); got != "https://mockapi.kiwoom.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestNewClientFromEnv_키없음(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "")
	t.Setenv("KIWOOM_SECRET_KEY", "")
	if _, err := kiwoom.NewClientFromEnv(); err == nil {
		t.Error("환경변수가 없으면 에러여야 한다")
	}
}

// 사용자가 명시한 옵션이 환경변수를 이겨야 한다.
func TestNewClientFromEnv_옵션이_환경변수를_이긴다(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	t.Setenv("KIWOOM_ENV", "mock")

	c, err := kiwoom.NewClientFromEnv(kiwoom.WithBaseURL("https://example.test"))
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	if got := c.BaseURL(); got != "https://example.test" {
		t.Errorf("BaseURL = %q — 명시한 옵션이 이겨야 한다", got)
	}
}
