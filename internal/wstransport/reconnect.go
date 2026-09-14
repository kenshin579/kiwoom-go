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

// reconnectNotifyAfter 는 몇 번째 실패부터 구독자에게 알릴지다.
//
// 첫 한두 번의 실패는 흔하고 곧 회복된다 — 그때마다 알리면 소음이 되고, 소음은 결국
// 무시된다. 그보다 오래 끌면 이야기가 달라진다: 아무 말도 없으면 "장이 조용하다" 와
// "한 시간째 못 붙고 있다" 가 구분되지 않는다. 그때부터는 **시도마다** 알린다.
const reconnectNotifyAfter = 3

// reconnectNotifyAfterDelay 는 횟수와 무관하게 이만큼 끌면 알리는 기준이다.
// 재연결 간격이 길게 설정된 경우에도 침묵이 오래가지 않게 한다.
const reconnectNotifyAfterDelay = 5 * time.Second

// pendingGapRetry 는 래치에 남은 구멍을 다시 흘려 보는 간격이다.
//
// deliver 만으로는 모자란다 — 다음 실시간이 와야 흘릴 수 있는데, 그 종목이 조용하면
// 구멍 알림이 그 침묵에 묻힌다. 자리가 나는 첫 순간에 흘리려면 따로 두드려야 한다.
const pendingGapRetry = 50 * time.Millisecond

// reconnectLoop 은 연결이 끊긴 뒤 다시 붙고 등록을 다시 보낸다.
//
// 재연결은 **받기만 하는 쪽**이라 2단계의 "재시도하지 않는다"와 어긋나지 않는다 —
// 실시간 등록을 다시 보내는 것은 주문을 두 번 넣지 않는다. 조건검색 요청(Request)은
// 다시 보내지 않는다.
//
// 붙을 때까지 조용히 있지 않는다. 몇 번 연달아 실패하면 시도마다 *ReconnectingError 를
// 흘린다 — GapError 는 결국 붙었을 때만 오므로, 장애가 이어지는 동안은 이것이 유일한
// 신호다.
//
// life 는 연결의 수명이다. Close 가 이것을 끊으면 재연결도 멈춘다.
func (c *Conn) reconnectLoop(life context.Context, since time.Time) {
	delay := c.retryDelay
	if delay <= 0 {
		delay = defaultRetryDelay
	}
	attemptTimeout := c.connectTimeout
	if attemptTimeout <= 0 {
		attemptTimeout = connectAttemptTimeout
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	attempts := 0
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

		attempts++
		// 시도마다 시간 제한을 건다. 수명 ctx 를 그대로 물리면 다이얼이나 로그인 응답에
		// 한 번 물리는 순간 재연결이 영영 멈춘다 — 그 침묵은 아무도 깨우지 못한다.
		attempt, cancel := context.WithTimeout(life, attemptTimeout)
		// 재연결도 Connect 와 **같은 경로**를 탄다. 로그인이 거부되면 토큰을 버리고 한 번
		// 다시 시도한다 — 그러지 않으면 폐기된 토큰을 든 채 영원히 실패한다.
		err := c.connectWithLoginRetry(attempt)
		cancel()
		if err != nil {
			if life.Err() != nil {
				return // Close 가 끊은 것이다. 닫히는 구독에 알릴 것은 없다
			}
			if attempts >= reconnectNotifyAfter || time.Since(since) >= reconnectNotifyAfterDelay {
				c.notifyReconnecting(since, attempts, err)
			}
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
		// 바로 흘리지 못한 구멍은 래치에 남는다. 다음 실시간이 오기를 기다리지 않고
		// 따로 두드린다 — 재연결 직후는 재등록 버스트로 버퍼가 가장 잘 차는 순간이라,
		// 구멍 알림이 가장 필요한 때에 가장 잘 묻힌다.
		if c.notifyGap(since, time.Now()) > 0 {
			go c.drainPendingGaps(life)
		}
		return
	}
}

// notifyGap 은 모든 구독자에게 끊긴 구간을 알린다. 아직 흘리지 못하고 래치에 남은
// 구독 수를 돌려준다.
//
// 조용히 넘기지 않는 이유: 사용자가 구멍을 알아야 REST 로 메꿀지 정할 수 있다.
// 이것을 감추면 "왜 어떤 날은 데이터가 비지?" 를 아무도 설명하지 못하게 된다.
//
// 자리가 없으면 **버리지 않고 래치에 얹는다.** 예전에는 dropped 를 올렸는데, 그러면
// 사용자는 *SlowConsumerError("느려서 버렸다") 만 보고 끊겼다는 사실은 영영 모른다 —
// 게다가 그 말은 사실도 아니다. 구멍 여럿이 겹치면 구간을 합친다(가장 이른 Since,
// 가장 늦은 Until).
//
// 보내는 것까지 subMu 안에서 한다. closeSubs 가 "지도에서 빼기 + 채널 닫기" 를 한 락
// 안에서 하므로, 락 밖에서 보내면 해지와 겹치는 순간 닫힌 채널에 보내다 죽는다.
// 송신은 전부 논블로킹이라 락을 들고 있어도 막히지 않는다(deliver 와 같은 규칙).
func (c *Conn) notifyGap(since, until time.Time) int {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	pending := 0
	// 한 채널에 여러 종목이 실려 있으면 한 번만 알린다. 래치도 그 하나에만 얹는다 —
	// 같은 채널에 여러 자리가 각자 래치를 들면 구멍 하나가 여러 번 온다.
	seen := map[chan Delivery]bool{}
	for _, list := range c.subs {
		for _, s := range list {
			if s.closed || seen[s.ch] {
				continue
			}
			seen[s.ch] = true
			s.latchGap(since, until)
			if !s.flushGap() {
				pending++
			}
		}
	}
	return pending
}

// drainPendingGaps 는 래치에 남은 구멍을 자리가 날 때까지 두드린다.
//
// life 가 끝나거나(Close) 전부 흘리면 끝난다 — 남겨 둘 이유가 없어지면 바로 빠진다.
func (c *Conn) drainPendingGaps(life context.Context) {
	t := time.NewTicker(pendingGapRetry)
	defer t.Stop()
	for {
		select {
		case <-life.Done():
			return
		case <-t.C:
		}
		if c.flushPendingGaps() == 0 {
			return
		}
	}
}

// flushPendingGaps 는 래치에 남은 구멍을 흘려 본다. 아직 남은 수를 돌려준다.
func (c *Conn) flushPendingGaps() int {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	pending := 0
	for _, list := range c.subs {
		for _, s := range list {
			if s.closed || s.pendingGap == nil {
				continue
			}
			if !s.flushGap() {
				pending++
			}
		}
	}
	return pending
}

// notifyReconnecting 은 아직 못 붙었다는 것을 모든 구독자에게 알린다.
//
// 래치하지 않는다 — 붙을 때까지 시도마다 되풀이해 오므로, 이번에 자리가 없으면 다음
// 것이 닿는다. dropped 로도 세지 않는다. 이것은 버려진 **데이터**가 아니라 상태 통보다.
//
// subMu 규칙은 notifyGap 과 같다.
func (c *Conn) notifyReconnecting(since time.Time, attempts int, last error) {
	e := &ReconnectingError{Since: since, Attempts: attempts, Last: last}
	now := time.Now()

	c.subMu.Lock()
	defer c.subMu.Unlock()

	seen := map[chan Delivery]bool{}
	for _, list := range c.subs {
		for _, s := range list {
			if s.closed || seen[s.ch] {
				continue
			}
			seen[s.ch] = true
			select {
			case s.ch <- Delivery{Err: e, Time: now}:
			default:
			}
		}
	}
}
