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

func TestNewClientFromEnv_대소문자를_가리지_않는다(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	for _, v := range []string{"mock", "MOCK", "Mock", " mock "} {
		t.Setenv("KIWOOM_ENV", v)
		c, err := kiwoom.NewClientFromEnv()
		if err != nil {
			t.Fatalf("KIWOOM_ENV=%q: %v", v, err)
		}
		if got := c.BaseURL(); got != "https://mockapi.kiwoom.com" {
			t.Errorf("KIWOOM_ENV=%q → BaseURL = %q", v, got)
		}
	}
}

func TestNewClientFromEnv_알수없는값은_에러다(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	// 오타를 조용히 운영으로 떨어뜨리면 모의투자로 믿고 실거래 서버를 친다
	for _, v := range []string{"moc", "mockk", "production", "1"} {
		t.Setenv("KIWOOM_ENV", v)
		if _, err := kiwoom.NewClientFromEnv(); err == nil {
			t.Errorf("KIWOOM_ENV=%q 는 에러여야 한다", v)
		}
	}
}

func TestNewClientFromEnv_prod와_미설정은_운영이다(t *testing.T) {
	t.Setenv("KIWOOM_APP_KEY", "AK")
	t.Setenv("KIWOOM_SECRET_KEY", "SK")
	for _, v := range []string{"", "prod", "PROD"} {
		t.Setenv("KIWOOM_ENV", v)
		c, err := kiwoom.NewClientFromEnv()
		if err != nil {
			t.Fatalf("KIWOOM_ENV=%q: %v", v, err)
		}
		if got := c.BaseURL(); got != "https://api.kiwoom.com" {
			t.Errorf("KIWOOM_ENV=%q → BaseURL = %q", v, got)
		}
	}
}

func TestNewClient_빈_BaseURL은_운영으로_되돌린다(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithBaseURL(""))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.BaseURL(); got != "https://api.kiwoom.com" {
		t.Errorf("BaseURL = %q — 빈 값이면 요청이 상대 경로로 나간다", got)
	}
}
