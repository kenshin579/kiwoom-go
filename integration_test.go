//go:build integration

// 실행:
//
//	KIWOOM_APP_KEY=... KIWOOM_SECRET_KEY=... go test -tags integration -run TestIntegration -v ./
//
// **운영 서버를 친다.** 읽기 전용만 쓴다 — 주문 패키지는 integration_guard_test.go 가
// 구조로 막는다(그 가드는 빌드 태그가 없어 항상 돈다).
//
// 목적은 커버리지가 아니라 **가정 검증**이다. 335개를 다 부르지 않는다. 3단계에서 스펙 표가
// 네 번 틀렸고(REG 의 item·type 이 배열, values 가 맵, trnm 이 GCNSRREQ, usa20290 의 FID),
// 그때마다 가려 준 것은 실제 예제였다. 여기서 묻는 것은 둘뿐이다:
//
//  1. 스펙이 서버와 맞나 — 특히 연속조회·return_code 형태·LIST 중첩·빈 문자열
//  2. 실시간이 실제로 오나
//
// 알아낸 것 중 스펙과 어긋나는 것은 tools/spec/SOURCE.md 의 표에 넣는다.
package kiwoom_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	kiwoom "github.com/kenshin579/kiwoom-go"
	"github.com/kenshin579/kiwoom-go/domestic/account"
	"github.com/kenshin579/kiwoom-go/domestic/chart"
	"github.com/kenshin579/kiwoom-go/domestic/quote"
	"github.com/kenshin579/kiwoom-go/domestic/stock"
)

const (
	// 삼성전자. 어느 시간대에도 데이터가 있고 상장폐지 걱정이 없다.
	probeSymbol = "005930"

	// throttle 은 호출 사이 간격이다.
	//
	// 이 라이브러리는 재시도를 하지 않고 스로틀링을 호출자 책임으로 뒀다(README 참고).
	// 하네스가 그 책임을 진다. 키움이 한도를 문서로 밝히지 않아 보수적으로 잡는다.
	throttle = 300 * time.Millisecond
)

// newClient 는 통합 테스트용 클라이언트를 만든다. 키가 없으면 **건너뛴다**(실패가 아니다).
//
// CI 가 키 없이 돌아도 조용해야 한다. 키가 없는 것은 실패가 아니라 "여기서는 확인할 수
// 없다" 이고, 그 둘을 섞으면 진짜 실패가 묻힌다.
func newClient(t *testing.T) *kiwoom.Client {
	t.Helper()
	if os.Getenv("KIWOOM_APP_KEY") == "" || os.Getenv("KIWOOM_SECRET_KEY") == "" {
		t.Skip("KIWOOM_APP_KEY / KIWOOM_SECRET_KEY 가 없다 — 통합 테스트를 건너뛴다")
	}
	// KIWOOM_ENV 를 명시적으로 확인한다. mock 이면 이 테스트의 전제(운영 서버가 정답을
	// 갖고 있다)가 깨지므로 건너뛰는 편이 낫다 — 조용히 다른 서버를 치고 "통과" 하면
	// 아무것도 검증하지 않은 것이다.
	if env := os.Getenv("KIWOOM_ENV"); env != "" && env != "prod" {
		t.Skipf("KIWOOM_ENV=%q — 이 테스트는 운영 서버를 전제로 한다", env)
	}

	c, err := kiwoom.NewClientFromEnv()
	if err != nil {
		t.Fatalf("NewClientFromEnv: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// report 는 값이 아니라 **모양**을 찍는다.
//
// 계좌 응답에는 실제 보유 종목이 들어 있다. 테스트 로그에 잔고가 남을 이유가 없다.
func report(t *testing.T, what string, n int, sample string) {
	t.Helper()
	t.Logf("%-28s 행 %d개  표본=%s", what, n, sample)
}

// TestIntegration_기본조회_응답이_스펙과_맞나 는 가장 단순한 모양을 본다.
//
// 여기서 깨지면 그 아래는 볼 것도 없다 — 인증·헤더·api-id 라우팅·return_code 처리가
// 전부 이 한 번에 걸린다.
func TestIntegration_기본조회_응답이_스펙과_맞나(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()

	res, meta, err := c.DomesticStock.GetDomesticStockInfo(ctx,
		stock.GetDomesticStockInfoRequest{StkCd: probeSymbol})
	if err != nil {
		t.Fatalf("GetDomesticStockInfo(ka10001): %v", err)
	}

	if res.StkCd == "" {
		t.Error("stk_cd 가 비어 있다 — 응답 모양이 스펙과 다르다")
	}
	if res.StkNm == "" {
		t.Error("stk_nm 이 비어 있다")
	}
	t.Logf("ka10001  종목=%s(%s)  연속조회=%q/%q", res.StkNm, res.StkCd, meta.ContYN, meta.NextKey)

	// 빈 문자열이 실제로 오는지 본다. 이 라이브러리는 모든 응답 필드를 string 으로 두고
	// 변환을 사용자에게 맡겼는데(README), 그 전제가 "없음 = 빈 문자열" 이다.
	var empties int
	for name, v := range map[string]string{
		"setl_mm": res.SetlMm, "fav": res.Fav, "cap": res.Cap, "crd_rt": res.CrdRt,
	} {
		if v == "" {
			empties++
			t.Logf("  빈 문자열: %s", name)
		}
	}
	t.Logf("  빈 필드 %d개 — kiwoom.DecimalOK 류가 0 과 가르는 대상", empties)
}

// TestIntegration_호가_LIST_중첩이_맞나 는 LIST 가 든 응답을 본다.
//
// 1단계에서 LIST 가 아닌 필드의 자식을 끌어올리던 버그가 있었고, 지금은 에러로 막는다.
// 실제 응답으로 그 트리가 맞는지 확인한 적은 없다.
func TestIntegration_호가_LIST_중첩이_맞나(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	time.Sleep(throttle)

	res, _, err := c.DomesticQuote.GetDomesticStockQuote(ctx,
		quote.GetDomesticStockQuoteRequest{StkCd: probeSymbol})
	if err != nil {
		t.Fatalf("GetDomesticStockQuote(ka10004): %v", err)
	}
	if res == nil {
		t.Fatal("응답이 nil 이다")
	}
	t.Logf("ka10004  호가 응답 수신 — 파싱 성공")
}

// TestIntegration_연속조회가_실제로_도나 는 cont-yn / next-key 왕복을 본다.
//
// **이것이 이 파일에서 가장 값진 확인이다.** 연속조회는 헤더로 오가는데(1단계 설계 §3),
// 첫 호출에 빈 헤더를 보내지 않는 것까지 코드가 신경 쓰고 있지만 실서버로 확인된 적이 없다.
func TestIntegration_연속조회가_실제로_도나(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	time.Sleep(throttle)

	req := chart.GetDomesticStockDailyChartRequest{
		StkCd:      probeSymbol,
		BaseDt:     time.Now().Format("20060102"),
		UpdStkpcTp: "1",
	}

	first, meta, err := c.DomesticChart.GetDomesticStockDailyChart(ctx, req)
	if err != nil {
		t.Fatalf("1회차 GetDomesticStockDailyChart(ka10081): %v", err)
	}
	n1 := len(first.StkDtPoleChartQry)
	if n1 == 0 {
		t.Fatal("일봉이 한 건도 없다 — 종목코드나 기준일자가 틀렸을 수 있다")
	}
	report(t, "ka10081 1회차", n1, first.StkDtPoleChartQry[0].Dt)
	t.Logf("  cont-yn=%q next-key=%q", meta.ContYN, meta.NextKey)

	if meta.ContYN != "Y" {
		t.Skip("서버가 이어볼 것이 없다고 한다(cont-yn != Y) — 연속조회 왕복은 확인하지 못했다")
	}

	time.Sleep(throttle)
	second, meta2, err := c.DomesticChart.GetDomesticStockDailyChart(ctx, req, kiwoom.WithCont(meta))
	if err != nil {
		t.Fatalf("2회차(연속조회): %v", err)
	}
	n2 := len(second.StkDtPoleChartQry)
	if n2 == 0 {
		t.Fatal("연속조회가 빈 결과를 줬다 — next-key 가 전달되지 않았을 수 있다")
	}
	report(t, "ka10081 2회차", n2, second.StkDtPoleChartQry[0].Dt)
	t.Logf("  cont-yn=%q next-key=%q", meta2.ContYN, meta2.NextKey)

	// 같은 페이지가 다시 오면 이어보기가 안 된 것이다.
	if first.StkDtPoleChartQry[0].Dt == second.StkDtPoleChartQry[0].Dt {
		t.Errorf("1·2회차의 첫 일자가 같다(%s) — 연속조회가 이어지지 않았다",
			first.StkDtPoleChartQry[0].Dt)
	}
}

// TestIntegration_계좌조회_모양만_본다 는 인증이 계좌 API 까지 통하는지 확인한다.
//
// **값을 찍지 않는다.** 실제 보유 종목이 돌아오므로 테스트 로그에 잔고가 남으면 안 된다.
// 모양(호출 성공·행 수)만 본다.
func TestIntegration_계좌조회_모양만_본다(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	time.Sleep(throttle)

	_, _, err := c.DomesticAccount.GetDomesticAccountEvaluationBalance(ctx,
		account.GetDomesticAccountEvaluationBalanceRequest{QryTp: "1", DmstStexTp: "KRX"})
	if err != nil {
		// 계좌 권한이 없거나 계좌가 비어 있을 수 있다. 업무 오류면 그 사실만 남긴다.
		var ae *kiwoom.APIError
		if errors.As(err, &ae) {
			t.Logf("kt00018  업무 오류 return_code=%d — 권한·계좌 상태를 확인하라", ae.ReturnCode)
			t.Skip("계좌 조회가 업무 오류를 냈다. 전송 계층은 정상 동작했다(에러를 제대로 갈라냈다)")
		}
		t.Fatalf("GetDomesticAccountEvaluationBalance(kt00018): %v", err)
	}
	t.Log("kt00018  계좌 조회 성공 — 값은 일부러 찍지 않는다")
}

// TestIntegration_실시간이_실제로_오나 는 이 파일의 두 번째 목적이다.
//
// 3단계 전체가 가짜 WS 서버로만 검증됐다. 실서버가 정말 로그인·등록을 받아 주고 푸시를
// 보내는지 확인한 적이 없다.
//
// **장 시간이 아니면 아무것도 오지 않는다.** 그때는 실패가 아니라 "확인하지 못했다" 다 —
// 연결·로그인·등록까지 갔으면 그 자체가 절반의 수확이다.
func TestIntegration_실시간이_실제로_오나(t *testing.T) {
	c := newClient(t)
	time.Sleep(throttle)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ch, err := c.DomesticRealtime.SubscribeDomesticStockTrade(ctx, probeSymbol)
	if err != nil {
		t.Fatalf("SubscribeDomesticStockTrade(0B): %v — 연결·로그인·등록 중 하나가 막혔다", err)
	}
	t.Log("0B  구독 성공 — 연결·로그인·REG 가 실서버에서 통했다")

	var got int
	for ev := range ch {
		if ev.Err != nil {
			var gap *kiwoom.GapError
			var rec *kiwoom.ReconnectingError
			switch {
			case errors.As(ev.Err, &gap):
				t.Logf("  구멍: %s ~ %s", gap.Since.Format(time.TimeOnly), gap.Until.Format(time.TimeOnly))
			case errors.As(ev.Err, &rec):
				t.Logf("  재연결 중: %d회차", rec.Attempts)
			default:
				t.Logf("  에러: %v", ev.Err)
			}
			continue
		}
		got++
		if got == 1 {
			t.Logf("  첫 푸시: 종목=%s 체결시간=%s 현재가=%s (Raw %d개)",
				ev.Symbol, ev.Value.TradeTime, ev.Value.CurrentPrice, len(ev.Raw))
			// 표에 없는 FID 가 오는지 본다 — 오면 tools/gen/fids.go 를 갱신해야 한다.
			if len(ev.Raw) > 0 {
				t.Logf("  받은 FID %d개", len(ev.Raw))
			}
		}
		if got >= 3 {
			cancel()
		}
	}

	if got == 0 {
		t.Skip("푸시가 오지 않았다 — 장 시간이 아니거나 해당 종목에 체결이 없다. " +
			"구독 자체는 성공했으므로 연결·로그인·등록 경로는 확인됐다")
	}
	t.Logf("0B  푸시 %d건 수신 — 실시간 경로 전체가 실서버에서 확인됐다", got)
}
