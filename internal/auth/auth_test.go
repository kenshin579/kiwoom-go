package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kenshin579/kiwoom-go/internal/auth"
)

func server(t *testing.T, expires string, calls *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("경로 = %s", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "client_credentials" || body["appkey"] != "AK" || body["secretkey"] != "SK" {
			t.Errorf("본문 = %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": "T1", "token_type": "bearer", "expires_dt": expires, "return_code": 0,
		})
	}))
}

func TestToken_캐시한다(t *testing.T) {
	var calls int32
	future := time.Now().In(time.FixedZone("KST", 9*3600)).Add(time.Hour).Format("20060102150405")
	srv := server(t, future, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	for i := 0; i < 3; i++ {
		tok, err := s.Token(context.Background())
		if err != nil || tok != "T1" {
			t.Fatalf("Token = %q, %v", tok, err)
		}
	}
	if calls != 1 {
		t.Errorf("발급 호출 = %d, want 1 (캐시돼야 한다)", calls)
	}
}

func TestToken_만료전에_갱신한다(t *testing.T) {
	var calls int32
	// 30초 뒤 만료 — 60초 여유보다 짧으므로 매번 새로 받아야 한다
	soon := time.Now().In(time.FixedZone("KST", 9*3600)).Add(30 * time.Second).Format("20060102150405")
	srv := server(t, soon, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	_, _ = s.Token(context.Background())
	_, _ = s.Token(context.Background())
	if calls != 2 {
		t.Errorf("발급 호출 = %d, want 2 (만료 임박이면 갱신)", calls)
	}
}

func TestToken_Invalidate(t *testing.T) {
	var calls int32
	future := time.Now().In(time.FixedZone("KST", 9*3600)).Add(time.Hour).Format("20060102150405")
	srv := server(t, future, &calls)
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	_, _ = s.Token(context.Background())
	s.Invalidate()
	_, _ = s.Token(context.Background())
	if calls != 2 {
		t.Errorf("발급 호출 = %d, want 2 (Invalidate 후 재발급)", calls)
	}
}

func TestToken_발급실패(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"return_code":3,"return_msg":"appkey 오류"}`))
	}))
	defer srv.Close()

	s := auth.New(srv.URL, "AK", "SK", srv.Client())
	if _, err := s.Token(context.Background()); err == nil {
		t.Fatal("에러여야 한다")
	}
}
