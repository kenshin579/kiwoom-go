// Package auth 는 키움 OAuth2 접근토큰을 발급·캐시한다.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// refreshMargin 은 만료 이 시간 전에 미리 갱신한다.
// 호출 도중 만료돼 401 을 받는 것보다, 조금 일찍 받는 편이 싸다.
const refreshMargin = 60 * time.Second

var kst = time.FixedZone("KST", 9*60*60)

// Source 는 토큰을 캐시하는 발급기. 동시 호출에 안전하다.
type Source struct {
	baseURL   string
	appKey    string
	secretKey string
	hc        *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// New 는 토큰 발급기를 만든다.
func New(baseURL, appKey, secretKey string, hc *http.Client) *Source {
	return &Source{baseURL: baseURL, appKey: appKey, secretKey: secretKey, hc: hc}
}

// Invalidate 는 캐시한 토큰을 버린다. 401 을 받았을 때 호출한다.
func (s *Source) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token, s.expiresAt = "", time.Time{}
}

// Token 은 유효한 토큰을 돌려준다. 없거나 만료가 임박했으면 새로 받는다.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Now().Before(s.expiresAt.Add(-refreshMargin)) {
		return s.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"grant_type": "client_credentials",
		"appkey":     s.appKey,
		"secretkey":  s.secretKey,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/oauth2/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")

	resp, err := s.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("kiwoom: 토큰 발급 요청 실패: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Token      string `json:"token"`
		ExpiresDt  string `json:"expires_dt"`
		ReturnCode int    `json:"return_code"`
		ReturnMsg  string `json:"return_msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("kiwoom: 토큰 응답 파싱 실패(HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || out.ReturnCode != 0 || out.Token == "" {
		return "", fmt.Errorf("kiwoom: 토큰 발급 실패(HTTP %d, return_code=%d): %s",
			resp.StatusCode, out.ReturnCode, out.ReturnMsg)
	}

	exp, err := time.ParseInLocation("20060102150405", out.ExpiresDt, kst)
	if err != nil {
		// 만료 시각을 못 읽으면 캐시하지 않는다 — 만료된 토큰을 계속 쓰는 것보다 낫다.
		return out.Token, nil
	}
	s.token, s.expiresAt = out.Token, exp
	return s.token, nil
}
