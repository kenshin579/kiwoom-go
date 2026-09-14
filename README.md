# kiwoom-go

키움증권 API(https://openapi.kiwoom.com) 의 Go 클라이언트. REST 와 실시간 WebSocket 을 함께 덮는다.

- 키움 API 는 337개라 손으로 쓸 수 없다 — 공식 기계 판독 스펙에서 코드를 **생성**한다.
- **337개 중 335개를 덮는다** — REST 304(국내주식 183 · 미국주식 121) + 실시간 WebSocket 23 +
  조건검색 8. 그중 333개가 생성물이고, 조건검색 둘(`ka10173`·`usa20290`)만 손으로 썼다.
- 응답 필드는 **전부 `string`**. 설명을 읽어 타입을 추론하면 틀린 값을 맞는 값처럼 보여주는
  실패가 된다 — 숫자·날짜 변환은 사용자가 `kiwoom.Decimal` 등으로 명시적으로 한다.
- **재시도 없음** — 401 도 재시도하지 않는다(주문이 두 번 들어갈 수 있다). 키움의 429/5xx 정책을
  모르는 채로 재시도하면 한도를 태운다.
- **구멍을 숨기지 않는다** — 실시간이 끊겼다 붙으면 그 사이를 놓친 것을 `*GapError` 로 알린다.
- 모의투자 지원(`KIWOOM_ENV=mock`).

## 설치

```bash
go get github.com/kenshin579/kiwoom-go@latest
```

Go 1.25+.

## 인증

1. [키움 Open API](https://openapi.kiwoom.com) 에서 앱키·시크릿키를 발급받는다.
2. 환경변수 `KIWOOM_APP_KEY`, `KIWOOM_SECRET_KEY` 를 설정한다. 모의투자를 쓰려면
   `KIWOOM_ENV=mock` 을 추가한다(`prod` 또는 미설정이면 운영). **알 수 없는 값은 에러다** —
   오타(`"moc"`)를 조용히 운영으로 떨어뜨리면 모의투자로 믿고 실거래 서버를 치게 된다.

```go
c, err := kiwoom.NewClientFromEnv()
// 또는
c, err := kiwoom.NewClient(appKey, secretKey)               // 운영
c, err := kiwoom.NewClient(appKey, secretKey, kiwoom.WithMock()) // 모의투자
```

`WithMock()` 은 REST 와 WebSocket 도메인을 **함께** 바꾼다. 하나만 바뀌면 사용자가 모의투자로
믿는 채로 실서버에 붙기 때문이다. 둘을 따로 가리켜야 하면 `WithBaseURL`(REST) ·
`WithWSBaseURL`(WebSocket) 을 쓴다.

WebSocket 을 한 번이라도 썼으면 다 쓰고 `c.Close()` 를 부른다 — 소켓과 재연결 루프가 남는다.
REST 만 쓴 클라이언트가 불러도 안전하다(붙은 적이 없으면 닫을 것도 없다).

## 지원 범위

| 갈래 | 개수 | 비고 |
|---|---|---|
| REST — 국내주식 | 183 | 생성 |
| REST — 미국주식 | 121 | 생성 |
| 실시간 WebSocket | 23 | 생성. 국내 19 · 미국 4 |
| 조건검색 | 8 | 생성 6 + 손으로 쓴 2(`ka10173`·`usa20290`) |
| **합계** | **335** | 생성 333 + 손으로 쓴 2 |

**남는 2개는 OAuth 접근토큰 발급·폐기다.**

- **발급**은 이미 동작한다 — `internal/auth` 가 손으로 다룬다. 토큰은 만료 60초 전에 스스로
  갱신되므로 사용자가 부를 일이 없어 공개 표면에 올리지 않았다.
- **폐기**는 만들지 않았다. 토큰은 만료로 정리되므로 부를 이유가 없고, 부르면 같은 토큰을
  쓰는 다른 호출이 함께 401 을 맞는다.

## 첫 호출

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/kenshin579/kiwoom-go"
	"github.com/kenshin579/kiwoom-go/domestic/stock"
)

func main() {
	c, err := kiwoom.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx := context.Background()
	resp, meta, err := c.DomesticStock.GetDomesticStockInfo(ctx, stock.GetDomesticStockInfoRequest{
		StkCd: "005930",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp.StkNm, kiwoom.Decimal(resp.CurPrc))
	_ = meta
}
```

## 구성

API 그룹마다 서브패키지가 하나다 — 29개(국내 17 · 미국 12). 국내는
`domestic/quote`·`chart`·`stock`·`account`·`order`·`realtime`·`condition` 등, 미국은
`overseas/ranking`·`stock`·`account`·`order`·`realtime`·`condition` 등이다. 요청/응답 타입도
그 패키지 안에 있다.

`kiwoom.Client` 가 하위 클라이언트 묶음을 임베딩하므로 **루트에서 바로 쓴다** —
`c.DomesticQuote`, `c.OverseasRanking`, `c.DomesticRealtime` 처럼. 전체 목록은 생성물
[`subclients.go`](subclients.go) 에 있다.

```go
import (
	"github.com/kenshin579/kiwoom-go"
	"github.com/kenshin579/kiwoom-go/overseas/ranking"
)

// ...

resp, meta, err := c.OverseasRanking.GetOverseasChangeRateTopByPeriodEtf(ctx,
	ranking.GetOverseasChangeRateTopByPeriodEtfRequest{StexTp: "0", Tm: "1"})
```

## 실시간 구독

```go
import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/kenshin579/kiwoom-go"
)

// ...

ctx, cancel := context.WithCancel(context.Background())
defer cancel() // 취소하면 서버에 해지(REMOVE)를 보내고 채널을 닫는다

ch, err := c.DomesticRealtime.SubscribeDomesticStockTrade(ctx, "005930")
if err != nil {
	return err
}
for ev := range ch {
	var gap *kiwoom.GapError
	if errors.As(ev.Err, &gap) {
		// 끊겼다 붙었다. 그 사이 이벤트는 받지 못했다 — 필요하면 REST 로 메꿔라.
		log.Printf("실시간 구멍: %s ~ %s", gap.Since, gap.Until)
		continue
	}
	if ev.Err != nil {
		return ev.Err
	}
	fmt.Println(ev.Symbol, ev.Value.CurrentPrice, ev.Value.TradeTime)
	// 표에 없는 FID 도 버리지 않는다.
	fmt.Println(ev.Raw["10"])
}
```

`Subscribe...` 는 종목을 여러 개 받는다(`Subscribe...(ctx, "005930", "000660")`). 한 번에
등록하고, 같은 채널로 섞여 온다 — `ev.Symbol` 로 가른다.

**연결은 시장마다 하나다**(국내·미국). 여러 구독이 한 소켓에 실리고 멀티플렉싱은 감춰져 있다.
연결은 **첫 구독 때 붙는다** — REST 만 쓰는 사용자는 소켓을 열지 않는다.

**이벤트 봉투는 `kiwoom.Event[T]` 하나다**(`stream.Event[T]` 의 별칭). 국내든 미국이든 같은
타입이라 제네릭 핸들러 하나로 받는다:

```go
func handle[T any](ev kiwoom.Event[T]) { /* ... */ }

handle(<-domCh) // domestic/realtime
handle(<-ovsCh) // overseas/realtime
```

봉투에는 `Symbol`·`Name`·`Time`·`Value`·`Raw`·`Err` 이 있다. `Err` 이 nil 이 아니면 `Value` 와
`Raw` 는 비어 있다 — 그 봉투는 데이터가 아니라 신호다.

**`Raw` 가 따로 있는 이유.** 실시간 값의 식별자는 FID 숫자이고, `Value` 의 Go 필드는
`tools/gen/fids.go` 의 이름표를 거쳐 나온다. 키움이 FID 를 새로 보내기 시작하면 그 값은
구조체에 자리가 없다 — `encoding/json` 이 조용히 버릴 자리다. 그래서 같은 본문을 **두 번**
푼다: 구조체로 한 번, `map[string]string` 으로 한 번. 표를 갱신하기 전까지도
`ev.Raw["1234"]` 로 꺼낼 수 있다.

## 구멍을 숨기지 않는다

실시간에서 가장 비싼 실패는 에러가 아니라 **침묵**이다. 이 라이브러리는 못 받은 것을 말한다.

- **끊기면 자동으로 다시 붙고 등록(REG)을 다시 보낸다.** 그러나 **그 사이 이벤트는 영영 받지
  못한다** — 키움 WebSocket 은 지나간 것을 다시 주지 않는다.
- **그것을 `*kiwoom.GapError` 로 알린다**(`Since`·`Until`). 메꿀지는 호출자가 정한다.
  라이브러리가 대신 고르지 않는 이유는, 무엇으로 메꿔야 하는지가 종목·실시간 타입마다
  다르기 때문이다 — 체결이면 분봉 차트일 수도 틱 차트일 수도 있고, 잔고면 계좌 조회다.
  틀린 REST 를 골라 메꾸면 구멍이 메워진 것처럼 보이는데 값은 틀려 있다.
- **계속 못 붙으면 `*kiwoom.ReconnectingError` 가 되풀이해 온다**(`Since`·`Attempts`·`Last`).
  `GapError` 는 **붙은 뒤에야** 오므로, 이것이 없으면 "장이 조용하다" 와 "한 시간째 못 붙고
  있다" 가 구분되지 않는다. 첫 한두 번은 알리지 않는다 — 흔하고 곧 회복되며, 소음은 결국
  무시된다. 그보다 오래 끌면 시도마다 알린다.
- **채널이 가득 차면 `*kiwoom.SlowConsumerError`**(`Type`·`Item`·`Dropped`). 이벤트를 버리고
  알린다 — 한 구독이 느리다고 다른 구독을 굶기지 않는다. 채널 버퍼는 구독당 256이다.
  밀린 알림이 몰릴 때는 **구멍이 먼저, 느림이 나중에** 간다. 자리가 한 칸뿐일 때 하나만
  통과한다면 무거운 쪽이 통과해야 한다.
- **해석하지 못한 프레임은 재연결하지 않고 센다.** `c.BadFrames()` 로 읽는다. 깨진 JSON 한
  건으로 재연결해 봐야 되찾는 것이 없고, 서버가 우리가 모르는 메시지를 계속 보내면 재연결
  핫 루프가 된다. 이 수가 0 이 아니고 계속 는다면 조용히 버려지는 실시간이 있다는 뜻이다 —
  "왜 어떤 이벤트는 안 오지?" 를 설명할 수 있는 유일한 자리다.

네 신호 모두 `errors.As` 로 가린다. 타입 별칭이라 `internal/` 을 import 하지 않고도 된다.

```go
var gap *kiwoom.GapError
var slow *kiwoom.SlowConsumerError
var rec *kiwoom.ReconnectingError
switch {
case errors.As(ev.Err, &gap):  // 끊겼다 붙었다 — 그 사이를 놓쳤다
case errors.As(ev.Err, &rec):  // 아직 못 붙었다 — 지금은 아무것도 오지 않는다
case errors.As(ev.Err, &slow): // 내가 느려서 버려졌다 — 채널을 더 빨리 비워라
case ev.Err != nil:            // 그 밖의 실패
}
```

## 조건검색

조건검색은 넷씩 짝이다 — 목록 조회, 요청(일반), 요청(실시간), 실시간 해제. 국내 4 + 미국 4 = 8.

**요청(실시간)만 응답과 채널을 함께 준다.** 조회 결과가 한 번 오고, 그 뒤로 편입·이탈 푸시가
이어지기 때문이다. 337개 중 이 둘만 그렇다.

```go
import (
	"github.com/kenshin579/kiwoom-go"
	"github.com/kenshin579/kiwoom-go/domestic/condition"
)

// ...

// 어떤 조건식이 있는지
list, err := c.DomesticCondition.ListDomesticConditionSearches(ctx,
	condition.ListDomesticConditionSearchesRequest{})

// 조회 결과 + 편입·이탈 푸시
resp, ch, err := c.DomesticCondition.RequestDomesticRealtimeConditionSearch(ctx,
	condition.RequestDomesticRealtimeConditionSearchRequest{
		Seq: "001", SearchType: "1", StexTp: "K",
	})
if err != nil {
	return err
}
for _, it := range resp.Data { // 조회 시점의 편입 종목
	fmt.Println(it.Jmcode)
}
for ev := range ch { // 그 뒤의 편입(I)·이탈(D)
	if ev.Err != nil {
		return ev.Err
	}
	fmt.Println(ev.Value.StockOrSectorCode, ev.Value.InsertDeleteType, ev.Value.TradeTime)
}

// 다 봤으면 해제한다. ctx 취소는 받기를 그만둘 뿐 서버에 해제를 보내지 않는다.
_, err = c.DomesticCondition.StopDomesticRealtimeConditionSearch(ctx, /* ... */)
```

**`ctx` 취소는 해제가 아니다.** 실시간 구독(`Subscribe...`)은 취소하면 REMOVE 를 보내지만,
조건검색은 `StopDomesticRealtimeConditionSearch`(ka10174) ·
`StopOverseasRealtimeConditionSearch`(usa20291) 로 따로 보내야 한다. 그러지 않으면 서버는
계속 밀어 보낸다.

**재연결은 조건검색 등록을 되살리지 못한다.** 실시간 구독은 REG 를 다시 보내면 되살아나지만
조건검색은 **요청 자체가 등록**이라 되살릴 REG 가 없다. 그래서 이 채널의 `*GapError` 는
"그 사이를 놓쳤다" 에 더해 **"지금은 아무것도 오지 않는다"** 는 뜻이다 — 받으면
`RequestDomesticRealtimeConditionSearch` 를 다시 불러 등록을 새로 세워야 한다.

**미국 조건검색(`usa20290`)의 푸시는 `Raw` 뿐이다.** `ev.Value`(`OverseasRealtimeConditionMatch`)
는 늘 영값이고 받은 FID 는 전부 `ev.Raw` 에 들어 있다.

**어떤 FID 가 오는지는 이 라이브러리도 모른다.** 국내 짝(`ka10173`)이 보내는 키를 미뤄
짐작할 수는 있지만 확인된 것이 아니므로 여기에 예시 키를 적지 않는다. 먼저 받은 것을
그대로 찍어 보고(`for k, v := range ev.Raw`) 무엇이 오는지 눈으로 확인한 뒤 쓰라.

이것은 **"푸시를 지원하지 않는다" 가 아니라 "이름표가 없다"** 다. 푸시는 실제로 오고 값도 다
들어 있다. 다만 스펙에 그 FID 들의 한글명 표가 통째로 비어 있어(공식 예제도 `COLUMNS = {}` 에
"수동 생성 필요" 마커를 달아 둔다) 어느 숫자가 무엇인지 문서가 말해 주지 않는다. 그래서 Go
필드 이름을 지어내지 않았다 — 스펙에 없는 것을 있는 것처럼 만들지 않는다. 337개 중 이 하나만
이렇다.

## 문자열 정책

응답 필드는 예외 없이 `string` 이다. 키움 스펙의 설명 문구("단위: 원", "부호가 포함된 숫자"
등)로 타입을 추론해 `int`/`decimal` 로 바로 내보내는 방식은, 설명이 틀리거나 애매한 필드에서
**틀린 값을 맞는 값처럼** 보여주는 실패로 이어진다. 그래서 변환은 사용자가 명시적으로 한다.
실시간 값(`ev.Value`)과 `ev.Raw` 도 같다.

```go
kiwoom.Decimal(s)   // 금액·수량 문자열 → decimal.Decimal (실패 시 0)
kiwoom.DecimalOK(s) // (decimal.Decimal, ok) — 빈 문자열은 (0, false)
kiwoom.Int(s)       // 정수 문자열 → int64 (실패 시 0)
kiwoom.IntOK(s)     // (int64, ok)
kiwoom.Date(s)      // YYYYMMDD → time.Time(KST)
kiwoom.DateTime(s)  // YYYYMMDDHHMMSS → time.Time(KST)
```

**빈 문자열은 "값 없음"이다**, "0"과 다르다. `Decimal`/`Int`/`Date`/`DateTime` 은 실패하면
조용히 제로값을 준다 — 값 없음과 파싱 실패를 구분해야 하면 `*OK` 변형을 쓴다.

`clean` 내부 헬퍼가 부호(`+1234`)·앞자리 0(`0012`)·표시용 콤마를 먼저 벗긴다.

## 연속조회

응답과 함께 돌아오는 `transport.Meta`(별칭 `kiwoom.Meta`)의 `ContYN` 이 `"Y"` 면 다음 페이지가
있다. `kiwoom.WithCont(meta)` 를 다음 호출에 넘겨 이어간다.

```go
resp, meta, err := c.DomesticQuote.GetDomesticStockQuote(ctx, req)
for err == nil && meta.ContYN == "Y" {
	resp, meta, err = c.DomesticQuote.GetDomesticStockQuote(ctx, req, kiwoom.WithCont(meta))
}
```

**자동 반복은 넣지 않았다** — 어떤 API 가 몇 페이지까지 가는지 모르는 채로 자동화하면
사용자가 모르는 사이에 호출 한도를 태운다. 언제 멈출지는 호출자가 정한다.

## 에러

```go
_, _, err := c.DomesticStock.GetDomesticStockInfo(ctx, req)
if err != nil {
	var ae *kiwoom.APIError
	if errors.As(err, &ae) {
		fmt.Println(ae.StatusCode, ae.ReturnCode, ae.ReturnMsg, ae.APIID)
	}
	if kiwoom.IsCode(err, 1505) { // 해당 API ID는 존재하지 않습니다
		// ...
	}
}
```

**키움은 업무 오류를 HTTP 200 + 본문 `return_code` 로 돌려준다.** 상태코드만 보면 실패를
놓친다 — `kiwoom.APIError` 가 HTTP 실패와 `return_code` 실패를 하나로 모아준다.

WebSocket 쪽에도 짝이 있다 — 로그인 거부는 `*kiwoom.WSLoginError`, 업무 오류는
`*kiwoom.WSAPIError` 다. `return_code` 가 int 로도 숫자 문자열로도 오는 것을 둘 다 받는다.

## 재시도

**재시도하지 않는다.** 401 을 받으면 토큰만 버리고 에러를 돌려주므로, 다음 호출이 새 토큰을
받아 스스로 회복된다.

같은 요청을 다시 보내지 않는 이유는 주문 때문이다 — 첫 요청이 서버에 닿아 체결됐는데 응답만
401 로 보이면 **같은 주문이 두 번** 들어간다. 어떤 API 가 돈을 움직이는지 스펙이 알려주지
않으므로 규칙을 하나만 둔다. 토큰은 만료 60초 전에 미리 갱신하므로 401 자체가 드물다.

429·5xx 도 재시도하지 않는다. 키움의 정책을 확인하기 전에 추측으로 재시도하면 한도를 더
빨리 태운다 — 속도 조절은 호출자 책임이다.

**실시간 WebSocket 은 이 규칙의 예외가 아니다.** 끊기면 다시 붙고 등록(REG)을 다시 보내지만,
실시간은 **받기만 하는 쪽**이라 재연결이 주문을 두 번 넣지 않는다. 반대로 조건검색
**요청**(`RequestDomesticRealtimeConditionSearch` 등)은 요청/응답이라 REST 와 같이
**재전송하지 않는다** — 그래서 재연결이 조건검색 등록을 되살리지 못하고, 그 사실을
`*GapError` 로 알린다(위 조건검색 절 참고).

재연결 간격은 고정 2초다. 지수 백오프를 쓰지 않는 이유는 실시간이 늦게 붙을수록 구멍이 커지고,
키움이 WS 재연결 한도를 문서로 밝히지 않았기 때문이다.

조회 API 처럼 다시 보내도 안전한 호출에 재시도가 필요하면 호출자가 직접 감싸면 된다:

```go
import (
	"errors"
	"net/http"
)

// ...

var ae *kiwoom.APIError
_, _, err := c.DomesticQuote.GetDomesticStockQuote(ctx, req)
if errors.As(err, &ae) && ae.StatusCode == http.StatusUnauthorized {
	// 토큰은 이미 버려졌다. 한 번 더 부르면 새 토큰으로 간다.
}
```

## 코드 생성

```bash
cd tools && go run ./gen/main
```

- 스펙은 `tools/spec/kiwoom_api_spec.json` 에 벤더링돼 있다(3.85MB). 출처와 갱신 방법은
  [`tools/spec/SOURCE.md`](tools/spec/SOURCE.md).
- 생성물(`domestic/**/*.go`, `overseas/**/*.go`, 루트 `subclients.go`)은 전부
  `// Code generated by tools/gen. DO NOT EDIT.` 로 시작한다. **생성물은 손으로 고치지 않는다** —
  고칠 게 있으면 **생성기**(`tools/gen/`)를 고치고 다시 생성한다. 손으로 고친 것은 다음 생성에서
  말없이 사라진다. 골든 테스트(`tools/gen/golden_test.go`)가 커밋된 생성물과 지금 스펙에서
  나오는 것을 바이트 단위로 비교해 이것을 강제한다.
- `tools/gen/fids.go` 는 **손으로 적는 표**다(401줄). 실시간 값의 식별자는 FID 숫자이고
  스펙에는 숫자와 한글명만 있다 — 영문명은 어디에도 없어서 자동 변환에 맡기면
  `N10`·`N9001` 같은 이름이 나온다. **표에 없는 FID 를 만나면 생성기가 멈춘다**(조용히
  건너뛰지 않는다). 실시간 필드 이름을 고칠 때는 생성물이 아니라 이 표를 고치고 다시 생성하라.
  한글명을 모든 줄에 남겨 둔 것도 같은 이유다 — 틀린 이름은 표를 훑으면 사람 눈에 보인다.
- **손으로 쓴 파일 2개**(`domestic/condition/realtime_condition_search.go` ·
  `overseas/condition/realtime_condition_search.go`)만 예외다. `ka10173` 은 337개 중 유일하게
  응답이 두 벌(조회 + 푸시)이라 스펙 본문이 갈라져 생성기의 트리 빌더가 접지 못하고,
  `usa20290` 은 푸시 FID 표가 스펙에 비어 있어 푸시를 `Raw` 로만 내야 한다. 둘을 위해 생성기에
  분기를 넣는 것보다 둘을 손으로 쓰고 생성기를 단순하게 두는 편이 낫다고 봤다. 두 파일에는
  생성물 헤더를 붙이지 않는다 — 붙이면 다음 생성 때 지워진다.
- `tools/` 는 별도 Go 모듈이다 — 벤더링한 스펙·생성기가 라이브러리 사용자의 의존성 그래프에
  섞이지 않는다.

## v0.1.0 이 보증하는 범위

이 버전은 **"생성되고, 컴파일되며, 스펙과 일치한다"** 까지만 보증한다. 즉:

- 337개 중 335개(REST 304 · 실시간 23 · 조건검색 8)에 대해, 스펙의 요청/응답 필드가 Go 구조체로
  정확히 옮겨졌고 전체가 `go build`/`go vet`/`go test -race` 를 통과한다.
- WebSocket 연결·로그인·PING·구독 라우팅·재연결·구멍 신호는 가짜 서버로 검증했다.

**실제 키움 서버에 호출해 응답을 검증하지는 않았다.** 필드 이름·타입(전부 string)·경로는
스펙에서 기계적으로 뽑은 것이고, 실시간 필드 이름은 거기에 더해 손으로 적은 표를 거쳤다.
스펙 자체의 오류나 실서버 동작과의 불일치는 아직 걸러지지 않았다. 실거래에 쓰기 전에 각 API 를
직접 호출해 확인할 것.

## License

MIT
