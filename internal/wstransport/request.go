package wstransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var errNotConnected = errors.New("kiwoom: websocket 이 연결되어 있지 않다")

// errDisconnected 는 응답을 기다리는 중에 연결이 끊겼다는 뜻이다.
//
// 재전송하지 않는다 — 조건검색은 요청/응답이라 REST 와 같은 규칙을 따른다. 대신
// 곧바로 알려 준다. 가만 두면 ctx 가 만료될 때까지 아무도 끊긴 줄을 모른다.
var errDisconnected = errors.New("kiwoom: 응답을 기다리는 중에 websocket 이 끊겼다 — 다시 보내려면 호출자가 직접 보내라")

// reqResult 는 대기자에게 가는 것이다. 응답이거나, 끊겼다는 통보다.
type reqResult struct {
	env envelope
	err error
}

// Request 는 패킷을 보내고 같은 trnm 의 응답을 기다린다.
//
// 짝짓기 열쇠가 trnm 뿐이다 — 키움 WS 에는 요청 식별자가 없다. 그래서 같은 trnm 의
// 요청을 동시에 두 건 보내면 응답이 뒤바뀔 수 있다. 조건검색은 그럴 일이 없지만,
// 그렇게 쓰지 못하도록 trnm 당 한 건만 대기시킨다.
//
// **재전송하지 않는다.** REST 와 같은 규칙이다(설계 §5).
func (c *Conn) Request(ctx context.Context, trnm string, body any, out any) error {
	key := strings.ToUpper(trnm)

	// Subscribe 와 같은 자리다 — 조건검색도 첫 호출이 연결을 연다.
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	waiter := make(chan reqResult, 1)
	c.reqMu.Lock()
	if c.reqs == nil {
		c.reqs = map[string]chan reqResult{}
	}
	if _, busy := c.reqs[key]; busy {
		c.reqMu.Unlock()
		return fmt.Errorf("kiwoom: %s 요청이 이미 대기 중이다 — 같은 trnm 을 동시에 보내지 마라", key)
	}
	c.reqs[key] = waiter
	c.reqMu.Unlock()

	defer func() {
		c.reqMu.Lock()
		delete(c.reqs, key)
		c.reqMu.Unlock()
	}()

	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return errNotConnected
	}
	if err := writeJSON(ctx, ws, body); err != nil {
		return err
	}

	select {
	case r := <-waiter:
		if r.err != nil {
			return r.err
		}
		env := r.env
		// 업무 오류는 return_code != 0 이다. REST 와 같다 — trnm 만 보면 실패를
		// 성공으로 읽는다.
		if env.ReturnCode != 0 {
			return &APIError{Trnm: env.Trnm, ReturnCode: env.ReturnCode.Int(), ReturnMsg: env.ReturnMsg}
		}
		if out != nil {
			if err := json.Unmarshal(env.raw, out); err != nil {
				return fmt.Errorf("kiwoom: %s 응답 파싱 실패: %w", key, err)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// APIError 는 업무 오류다. REST 의 transport.APIError 와 같은 자리다 —
// 성공·실패를 trnm 이 아니라 return_code 로 가른다.
type APIError struct {
	Trnm       string // 요청한 trnm(어느 요청이 실패했는지)
	ReturnCode int    // 본문 return_code
	ReturnMsg  string // 본문 return_msg
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kiwoom: %s 실패 (return_code=%d): %s", e.Trnm, e.ReturnCode, e.ReturnMsg)
}

// routeResponse 는 대기 중인 요청에 응답을 건넨다. 짝이 없으면 false.
func (c *Conn) routeResponse(env envelope) bool {
	c.reqMu.Lock()
	w, ok := c.reqs[strings.ToUpper(env.Trnm)]
	c.reqMu.Unlock()
	if !ok {
		return false
	}
	select {
	case w <- reqResult{env: env}:
	default:
	}
	return true
}

// failRequests 는 대기 중인 요청을 전부 깨운다. 연결이 끊겼을 때 부른다.
//
// 건드리지 않는 경우가 둘 있다.
//
// 하나는 이미 응답이 담긴 대기자다(버퍼가 차 있으면 default 로 빠진다) — 응답이
// 도착한 뒤에 끊긴 것을 실패로 뒤집으면 안 된다.
//
// 다른 하나는 여기서 가리지 못한다. onReadFailure 는 **세대를 보지 않고** 이것을
// 부르므로, 지난 세대의 읽기 루프가 뒤늦게 죽으면 그 사이 새로 붙은 연결에 건 요청까지
// 깨운다. 그래도 그대로 두는 쪽이 낫다 — 세대를 따져 건너뛰면, 진짜로 끊긴 대기자가
// 아무 통보 없이 ctx 만료까지 매달리는 경우가 생긴다. 조건검색은 재전송하지 않으므로
// 잘못 깨운 쪽은 호출자가 다시 보내면 되지만, 영영 못 깨운 쪽은 손쓸 방법이 없다.
func (c *Conn) failRequests(err error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	for _, w := range c.reqs {
		select {
		case w <- reqResult{err: err}:
		default:
		}
	}
}
