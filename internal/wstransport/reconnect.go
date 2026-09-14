package wstransport

import (
	"context"
	"time"
)

// defaultRetryDelay 는 재연결 간격이다.
//
// 지수 백오프를 쓰지 않는다 — 실시간은 늦게 붙을수록 구멍이 커지고, 키움이 WS 재연결
// 한도를 문서로 밝히지 않았다. 고정 간격이 예측 가능하고, 구멍은 GapError 로 드러난다.
const defaultRetryDelay = 2 * time.Second

// connectAttemptTimeout 은 재연결 한 번에 주는 시간이다.
//
// 연결의 수명 ctx 는 Close 전에는 끝나지 않으므로, 이것이 없으면 다이얼이나 로그인
// 응답에서 한 번 물렸을 때 재연결이 영영 멈춘다 — 구멍을 알릴 사람도 없어진다.
const connectAttemptTimeout = 10 * time.Second

// reconnectLoop 은 연결이 끊긴 뒤 다시 붙고 등록을 다시 보낸다.
//
// 재연결은 **받기만 하는 쪽**이라 2단계의 "재시도하지 않는다"와 어긋나지 않는다 —
// 실시간 등록을 다시 보내는 것은 주문을 두 번 넣지 않는다. 조건검색 요청(Request)은
// 다시 보내지 않는다.
//
// life 는 연결의 수명이다. Close 가 이것을 끊으면 재연결도 멈춘다.
func (c *Conn) reconnectLoop(life context.Context, since time.Time) {
	delay := c.retryDelay
	if delay <= 0 {
		delay = defaultRetryDelay
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-life.Done():
			return
		case <-timer.C:
		}
		timer.Reset(delay)

		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}

		attempt, cancel := context.WithTimeout(life, connectAttemptTimeout)
		err := c.connectOnce(attempt)
		cancel()
		if err != nil {
			continue
		}

		// 붙었으면 등록을 다시 보내고, 구멍을 알린다.
		//
		// 해지된 등록은 regs 에 없다(Subscribe 의 뒷정리가 먼저 뺀다). 남겨 두면
		// 구독자가 없는 종목을 계속 받게 된다.
		c.subMu.Lock()
		regs := append([]*registration(nil), c.regs...)
		c.subMu.Unlock()
		for _, r := range regs {
			sendCtx, cancel := context.WithTimeout(life, 5*time.Second)
			_ = c.sendReg(sendCtx, "REG", r.typ, r.items)
			cancel()
		}
		c.notifyGap(since, time.Now())
		return
	}
}

// notifyGap 은 모든 구독자에게 끊긴 구간을 알린다.
//
// 조용히 넘기지 않는 이유: 사용자가 구멍을 알아야 REST 로 메꿀지 정할 수 있다.
// 이것을 감추면 "왜 어떤 날은 데이터가 비지?" 를 아무도 설명하지 못하게 된다.
//
// 보내는 것까지 subMu 안에서 한다. closeSubs 가 "지도에서 빼기 + 채널 닫기" 를 한 락
// 안에서 하므로, 락 밖에서 보내면 해지와 겹치는 순간 닫힌 채널에 보내다 죽는다.
// 송신은 전부 논블로킹이라 락을 들고 있어도 막히지 않는다(deliver 와 같은 규칙).
func (c *Conn) notifyGap(since, until time.Time) {
	gap := &GapError{Since: since, Until: until}

	c.subMu.Lock()
	defer c.subMu.Unlock()

	// 한 채널에 여러 종목이 실려 있으면 한 번만 알린다.
	seen := map[chan Delivery]bool{}
	for _, list := range c.subs {
		for _, s := range list {
			if s.closed || seen[s.ch] {
				continue
			}
			seen[s.ch] = true
			select {
			case s.ch <- Delivery{Err: gap, Time: until}:
			default:
				// 자리가 없다. 버린 건수로 세어 두면 자리가 나는 첫 순간에
				// *SlowConsumerError 로 드러난다 — 조용히 사라지지는 않는다.
				s.dropped++
			}
		}
	}
}
