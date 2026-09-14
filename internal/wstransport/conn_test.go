package wstransport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestConn_로그인_패킷을_먼저_보낸다(t *testing.T) {
	f := newFakeServer(t)
	tok := &stubToken{token: "TKN"}

	c := New(f.wsURL(), "", tok)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	ps := f.packets()
	if len(ps) == 0 {
		t.Fatal("서버가 아무 패킷도 못 받았다")
	}
	if ps[0]["trnm"] != "LOGIN" {
		t.Errorf("첫 패킷 trnm = %v, 기대 LOGIN", ps[0]["trnm"])
	}
	if ps[0]["token"] != "TKN" {
		t.Errorf("token = %v, 기대 TKN", ps[0]["token"])
	}
}

func TestConn_로그인_실패는_에러다(t *testing.T) {
	f := newFakeServer(t)
	f.loginCode = 3 // 0 이 아니면 실패

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Connect(ctx)
	if err == nil {
		t.Fatal("로그인 실패인데 에러가 없다")
	}
	var le *LoginError
	if !errorsAs(err, &le) {
		t.Fatalf("에러 타입 = %T, 기대 *LoginError", err)
	}
	if le.ReturnCode != 3 {
		t.Errorf("ReturnCode = %d, 기대 3", le.ReturnCode)
	}
}

func TestConn_PING_을_그대로_되돌려_보낸다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, c *websocket.Conn) {
		_ = f.writeJSON(c, map[string]any{"trnm": "PING", "nonce": "abc"})
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	// PING echo 가 서버에 도착할 때까지 기다린다.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range f.packets() {
			if p["trnm"] == "PING" && p["nonce"] == "abc" {
				return // 통과
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("PING echo 가 오지 않았다")
}

// 연결의 수명이 Connect 의 ctx 와 분리돼 있는지 본다.
//
// Connect 에 200ms 짜리 ctx 를 준다. 그 ctx 를 수신 루프에 그대로 물렸다면 200ms 뒤에
// 루프가 죽고, 400ms 뒤에 오는 PING 에 아무도 답하지 않는다. 계획서의 함정 1번이 실제로
// 막혔는지 보는 유일한 방법이다.
func TestConn_수명은_Connect_의_ctx_와_다르다(t *testing.T) {
	f := newFakeServer(t)
	f.onConn = func(t *testing.T, n int, c *websocket.Conn) {
		time.Sleep(400 * time.Millisecond) // Connect 의 ctx 가 이미 만료된 뒤
		_ = f.writeJSON(c, map[string]any{"trnm": "PING", "nonce": "late"})
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range f.packets() {
			if p["trnm"] == "PING" && p["nonce"] == "late" {
				return // 통과 — ctx 만료 후에도 연결이 살아 PING 에 답했다
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("ctx 만료 뒤의 PING echo 가 오지 않았다 — 수신 루프가 Connect 의 ctx 에 묶여 있다")
}

func TestConn_Close_는_두_번_불러도_안전하다(t *testing.T) {
	f := newFakeServer(t)

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_ = c.Close()
	if err := c.Close(); err != nil {
		t.Fatalf("두 번째 Close: %v", err)
	}
	if f.connCount() != 1 {
		t.Errorf("연결 횟수 = %d, 기대 1", f.connCount())
	}
}

// 로그인 응답 전에 다른 메시지가 오면 실패해야 한다. 조용히 넘기면 로그인 실패를
// 성공으로 착각한다.
func TestConn_로그인_응답_전의_다른_메시지는_에러다(t *testing.T) {
	f := newFakeServer(t)
	f.beforeLoginAck = func(c *websocket.Conn) {
		_ = f.writeJSON(c, map[string]any{"trnm": "REAL", "data": []any{}})
	}

	c := New(f.wsURL(), "", &stubToken{token: "TKN"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		_ = c.Close()
		t.Fatal("로그인 응답 전에 REAL 이 왔는데 에러가 없다")
	}
}

// 인증 실패면 토큰을 버리고 로그인만 한 번 다시 시도한다. 요청 재전송이 아니라
// 연결 수립이므로 "같은 요청을 다시 보내지 않는다" 규칙과 어긋나지 않는다.
func TestConn_로그인_실패시_토큰을_버리고_한_번_재시도한다(t *testing.T) {
	f := newFakeServer(t)
	f.loginCode = 3
	tok := &stubToken{token: "TKN"}

	c := New(f.wsURL(), "", tok)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		_ = c.Close()
		t.Fatal("로그인 실패인데 에러가 없다")
	}
	if got := tok.invalidateCount(); got != 1 {
		t.Errorf("Invalidate 횟수 = %d, 기대 1", got)
	}
	if got := f.connCount(); got != 2 {
		t.Errorf("연결 횟수 = %d, 기대 2(최초 + 재시도 1회)", got)
	}
}

func errorsAs(err error, target any) bool { return errors.As(err, target) }
