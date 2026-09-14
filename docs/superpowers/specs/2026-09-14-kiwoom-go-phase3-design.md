# kiwoom-go 3단계 — 실시간 WebSocket 23 + 조건검색 8 설계

2026-09-14

## 1. 목표

- 남은 **31개**를 덮어 스펙 337개를 전부 지원한다.
- 실시간 푸시를 **타입이 살아 있는 채널**로 건넨다.
- 끊김을 자동으로 회복하되 **구멍을 숨기지 않는다**.
- 1·2단계의 규율을 그대로 지킨다 — 지어낸 이름은 표로 명시하고, 표에 없으면 멈춘다.

### 목표 아님

- **OAuth 접근토큰 폐기(1개)** — 토큰은 만료로 정리된다. 2단계와 같은 이유로 미룬다.
- **긴 설명의 한 줄 주석 재배치** — 2단계 생성물 8,373줄 중 200자 초과가 38줄(0.45%),
  500자 초과가 1줄이다. 실측 결과 거슬리지 않으므로 **여기서 닫는다.** 고치지 않는다.
- **모의투자 스모크 테스트·v0.1.0 태그** — 별개의 작업이다. 3단계가 끝난 뒤에 한다.
- **공식 Python 클라이언트를 벤더링하는 것** — 규약만 읽어 `SOURCE.md` 에 적는다.
  코드를 옮겨 오지 않는다(라이선스·유지보수 둘 다 이유다).

## 2. 범위 (실측, 2026-09-14)

31개 = 실시간 23(국내 19 · 미국 4) + 조건검색 8(국내 4 · 미국 4).

`api_names.json` 이 **이미 31개를 전부 덮는다**(`0B` → `subscribe_domestic_stock_trade`).
API 수준의 이름은 지어낼 것이 없다.

### 실시간 23종

`값` 열은 FID 값 필드 수다.

| type | 이름 | 값 | 공식 영문명 |
|---|---|---|---|
| `00` | 주문체결 | 35 | `subscribe_domestic_order_fill` |
| `04` | 잔고 | 27 | `subscribe_domestic_balance` |
| `0A` | 주식기세 | 19 | `subscribe_domestic_stock_surge` |
| `0B` | 주식체결 | 43 | `subscribe_domestic_stock_trade` |
| `0C` | 주식우선호가 | 2 | `subscribe_domestic_stock_best_quote` |
| `0D` | 주식호가잔량 | 163 | `subscribe_domestic_stock_order_book_depth` |
| `0E` | 주식시간외호가 | 5 | `subscribe_domestic_stock_after_hours_quote` |
| `0F` | 주식당일거래원 | 57 | `subscribe_domestic_stock_today_brokers` |
| `0G` | ETF NAV | 15 | `subscribe_domestic_etf_nav` |
| `0H` | 주식예상체결 | 7 | `subscribe_domestic_stock_expected_execution` |
| `0I` | 국제금환산가격 | 4 | `subscribe_domestic_international_gold_converted_price` |
| `0J` | 업종지수 | 12 | `subscribe_domestic_sector_index` |
| `0U` | 업종등락 | 14 | `subscribe_domestic_sector_change` |
| `0g` | 주식종목정보 | 10 | `subscribe_domestic_stock_info` |
| `0m` | ELW 이론가 | 10 | `subscribe_domestic_elw_theoretical_price` |
| `0s` | 장시작시간 | 3 | `subscribe_domestic_market_open_time` |
| `0u` | ELW 지표 | 6 | `subscribe_domestic_elw_indicator` |
| `0w` | 종목프로그램매매 | 17 | `subscribe_domestic_stock_program_trade` |
| `1h` | VI발동/해제 | 19 | `subscribe_domestic_vi_event` |
| `F4` | 미국주식 실시간 주문 확인 | 19 | `subscribe_overseas_order_confirmation` |
| `F5` | 미국주식 실시간 체결 | 36 | `subscribe_overseas_order_fill` |
| `FE` | 미국주식 실시간 체결가 | 18 | `subscribe_overseas_trade_price` |
| `FT` | 미국주식 10호가 | 65 | `subscribe_overseas_order_book` |

### 23개가 23개 API 가 아니다

23종의 **요청 모양이 전부 같다.** 스펙에서 요청 서명을 뽑아 세면 종류가 **1개**다:

```
trnm(서비스명, REG|REMOVE) · grp_no(그룹번호) · refresh(기존등록유지여부)
data[] LIST
  ├ item (실시간 등록 요소 — 거래소별 종목코드)
  └ type (실시간 항목 — TR명 0A,0B…)
```

달라지는 것은 `type` 코드와 **푸시로 받는 값 스키마**뿐이다. 그래서 이 23개는
"메서드 23개"가 아니라 **등록 메시지 하나 + 값 구조체 23벌**이다. 전송 계층은 한 벌이면 된다.

### 조건검색 8개

| api-id | 이름 | `trnm` |
|---|---|---|
| `ka10171` / `usa20280` | 조건검색 목록조회 | `CNSRLST` / `GCNSRLST` |
| `ka10172` / `usa20281` | 조건검색 요청 일반 | `CNSRREQ` / `GCNSRREQ` |
| `ka10173` / `usa20290` | 조건검색 요청 실시간 | `CNSRREQ` / `GCNSRREQ`, 이후 `REAL` 푸시 |
| `ka10174` / `usa20291` | 조건검색 실시간 해제 | `CNSRCLR` / `GCNSRCLR` |

`usa20290` 은 이름과 달리 응답에 실시간 푸시 절이 없다 — §8 참고.

미국 쪽은 `trnm` 에 **`G` 접두어**가 붙는다. 스펙에 미국 `조건검색 요청 일반` 의 `trnm` 이
`CNSRREQ` 와 `GCNSRREQ` 두 가지로 적힌 자리가 있다 — 문서 불일치이므로 실제 메시지로 확정한다.

## 3. 전송 — 연결 둘

호스트는 환경에 따라 둘, 경로는 시장에 따라 둘이다. 곱이 아니라 **환경 하나를 고르면 연결이 둘**이다.

| | 호스트 |
|---|---|
| 운영 | `wss://api.kiwoom.com:10000` |
| 모의투자 | `wss://mockapi.kiwoom.com:10000` |

| 경로 | 싣는 것 |
|---|---|
| `/api/dostk/websocket` | 국내 23개 (실시간 19 + 조건검색 4) |
| `/api/us/websocket` | 미국 8개 (실시간 4 + 조건검색 4) |

REST 의 `ProdBaseURL`/`MockBaseURL` 과 같은 짝이므로 `WithMock()` 이 그대로 따라온다.

`internal/wstransport` 를 새로 만든다. REST 의 `internal/transport` 와 형제이고 토큰 발급기
`internal/auth` 를 **공유한다**. 경로별로 연결 하나씩, 최대 둘. 연결은 **첫 구독에서 게으르게**
열고 구독이 모두 사라지면 닫는다 — REST 만 쓰는 사용자가 소켓을 들고 있을 이유가 없다.

한 소켓에 여러 구독이 실린다. 멀티플렉싱은 전송 계층 안에 감춘다 — 호출자는 자기 채널만 본다.

### 핸드셰이크 (확인 완료)

스펙 전체에서 `LOGIN`·`PING`·`PONG` 이 **0회**다. 스펙은 api-id 별 메시지만 문서화하고
연결을 여는 절차는 담고 있지 않다.

같은 커밋(`234560d`)의 공식 Python 클라이언트 `kiwoom/core/ws_client.py` 에서 확인했다.
근거를 `tools/spec/SOURCE.md` 에 파일·커밋과 함께 적는다.

1. `wss://<호스트>:10000` + 경로로 연결한다.
2. 로그인 패킷을 보낸다:
   ```json
   {"trnm": "LOGIN", "token": "<접근토큰>"}
   ```
3. **로그인 응답을 먼저 받아야 한다.** `trnm == "LOGIN"` 인 메시지가 오고,
   `return_code != 0` 이면 실패다. 로그인 응답 전에 **다른 메시지가 먼저 오면 에러**다.
4. **PING 은 그대로 되돌려 보낸다.** `trnm == "PING"` 인 메시지(또는 `"PING"` 문자열)를
   받으면 **받은 것을 그대로 echo** 한다. PING 은 수신 루프에서 걸러내고 호출자에게 올리지 않는다.
5. 로그인이 인증 오류로 실패하면 **토큰을 새로 받아 로그인만 한 번 다시** 시도한다.
   이것은 요청 재전송이 아니라 연결 수립이므로 §5 의 "재전송하지 않는다"와 어긋나지 않는다.

REG/REMOVE 패킷의 `item`·`type` 은 **배열**이다 — 스펙의 depth-1 `String` 선언과 다르다:

```json
{"trnm":"REG","grp_no":"1","refresh":"1","data":[{"item":["005930"],"type":["0B"]}]}
```

푸시(REAL)에서는 `item` 이 **스칼라**다. 등록과 수신의 모양이 다르다는 것을 놓치면 안 된다.

## 4. 구독 — 타입별 채널

```go
ch, err := c.DomesticRealtime.SubscribeDomesticStockTrade(ctx, "005930", "000660")
if err != nil { return err }
for ev := range ch {
    if ev.Err != nil {
        // 구멍 또는 종료
        continue
    }
    fmt.Println(ev.Symbol, ev.Value.CurrentPrice, ev.Value.TradeTime)
}
```

23개 메서드가 각각 자기 타입의 채널을 준다. 메서드 이름은 **공식 영문명을 그대로** 쓴다 —
`c.DomesticRealtime.SubscribeDomesticStockTrade` 는 중복돼 보이지만, 2단계가 이미
`c.OverseasRanking.GetOverseasChangeRateTopByPeriodEtf` 로 같은 중복을 감수했다. 337개의
이름 규칙은 예쁨보다 **예측 가능함**이 이긴다(1단계 설계 §6.1).

이벤트 봉투:

```go
type Event[T any] struct {
	Symbol string    // 실시간 등록 요소 (종목코드)
	Time   time.Time // 수신 시각
	Value  T         // 타입별 값 구조체
	Err    error     // 구멍·종료 신호. 이때 Value 는 영값이다
}
```

해지는 `ctx` 취소다 — 그때 `REMOVE` 를 보내고 채널을 닫는다. 채널은 **버퍼를 둔다**.
사용자가 느리면 버퍼가 차고, 차면 그 구독에 `*SlowConsumerError` 를 흘린다. 소켓 전체를
막지 않는다 — 한 구독의 느림이 다른 구독을 굶기면 안 된다.

## 5. 재연결 — 붙이되 구멍을 숨기지 않는다

끊기면 자동으로 다시 붙고 등록 메시지를 다시 보낸다. **그 사이 이벤트는 영영 못 받는다.**

그것을 조용히 넘기지 않는다. 끊긴 구간을 `*GapError` 로 만들어 이벤트의 `Err` 에 실어 보낸다:

```go
type GapError struct {
	Since time.Time // 끊긴 시각
	Until time.Time // 다시 붙은 시각
}
```

사용자는 이걸 보고 필요하면 REST 로 메꾼다. 라이브러리가 대신 메꿔 주지는 않는다 —
어떤 REST 호출로 메꿔야 하는지는 종목·타입마다 다르고, 우리가 고를 일이 아니다.

### 2단계의 "재시도하지 않는다"와 어긋나지 않는다

2단계는 전송 계층의 재시도를 **전부** 걷어냈다. 이유는 하나였다 — 첫 요청이 서버에 닿아
체결됐는데 응답만 401 로 보이면 **같은 주문이 두 번** 들어간다.

실시간은 **받기만 하는 쪽**이다. `주문체결(00)` 도 체결을 알려주는 채널이지 넣는 채널이
아니다. 재연결은 주문을 두 번 넣지 않는다. 그래서 같은 규칙을 들고 오지 않는다.

조건검색 요청(`CNSRREQ` 등)은 요청/응답이므로 **재전송하지 않는다** — REST 와 같은 규칙을
따른다. 재연결 후 자동으로 다시 보내는 것은 실시간 등록(`REG`)뿐이다.

## 6. FID 표 — 400개를 손으로

실시간 값 필드의 식별자는 **FID 숫자**다. 스펙에 영문명이 없다:

```json
{"element": "10", "depth": 2, "한글명": "현재가",   "type": "String"}
{"element": "20", "depth": 2, "한글명": "체결시간", "type": "String"}
```

서로 다른 FID **400개** — 실시간 398개에, `ka10173` 에만 나오는 2개(`841` 일련번호,
`843` 삽입삭제 구분)를 더한 값이다. 1·2단계의 `GoName` 을 그대로 먹이면 `Values.N10`·`Values.N9001` 이
된다(숫자로 시작하는 이름에 `N` 을 붙이는 1단계 규칙). 공개 라이브러리 표면으로 쓸 수 없다.

`tools/gen/fids.go` 에 `groups.go` 와 **같은 방식으로** 표를 둔다:

```go
// Fids 는 FID → Go 필드 이름 표다.
//
// **여기는 지어낸 이름이다.** groups.go 와 같은 이유로 손으로 적는다 — 스펙이 주는 것은
// FID 숫자와 한글명뿐이고, 영문명은 어디에도 없다. 한글명을 모든 줄에 남겨 사람이 훑을 수 있게 한다.
var Fids = []Fid{
	{"10",   "CurrentPrice", "현재가"},
	{"20",   "TradeTime",    "체결시간"},
	{"9001", "Symbol",       "종목코드"},
	// …
}
```

**표에 없는 FID 를 만나면 생성기가 멈춘다.** `groups.go` 의 카테고리와 같은 규율이다.

한글명이 갈리는 FID 가 **66개** 있다. 전부 표기 차이다(`매도1호가` vs `매도호가1`,
`시간` vs `체결시간`, `9001` 이 `종목코드`/`종목,업종코드`/`종목코드,업종코드`). 의미가 실제로
충돌하는 FID 는 없다. FID 당 이름 하나로 접고, 접었다는 사실과 버린 표기를 주석에 남긴다.

### 이름을 어떻게 짓나

한글명에서 옮긴다. 기계적으로 할 수 없으므로 초안을 쓰고 **검토를 따로 받는다.**
한글명이 모든 줄에 주석으로 남으므로, 틀린 이름은 표를 훑으면 사람 눈에 보인다.
2단계의 `groups.go`(25줄)와 같은 성격이되 규모가 크므로, 검토를 구현과 분리한다.

## 7. 값 컨테이너는 맵이다 (확인 완료)

같은 `values` 필드가 실시간 23곳에서는 `LIST`, `ka10173` 에서는 `Object` 로 선언돼 있다.
스펙이 스스로 모순이다.

공식 클라이언트 `kiwoom/realtime/decoders.py` 가 **맵**임을 확정해 준다. 문서화된 REAL 모양:

```json
{
  "trnm": "REAL",
  "data": [
    {"type": "0B", "name": "주식체결", "item": "005930",
     "values": {"10": "-82000", "15": "+12345"}}
  ]
}
```

`LIST` 쪽이 문서 오류다. `values` 는 **FID 문자열을 키로 하는 맵**이고, 생성물은 FID 를
`json` 태그로 받는다:

```go
type DomesticStockTrade struct {
	TradeTime    string `json:"20"` // 체결시간
	CurrentPrice string `json:"10"` // 현재가
	// …
	Raw map[string]string `json:"-"` // §7 참고
}
```

### 모르는 FID 를 버리지 않는다

공식 디코더는 **표에 없는 FID 를 그대로 통과시킨다**("unknown FID keys pass through
unchanged so no data is lost"). `encoding/json` 은 반대로 **조용히 버린다.**

그래서 값 구조체마다 `Raw map[string]string` 을 둔다. 먼저 맵으로 받아 구조체를 채우고,
**맵 전체를 `Raw` 에 그대로 남긴다.** 키움이 필드를 추가했는데 우리 표가 아직 모르면,
사용자는 `Raw["1234"]` 로 꺼낼 수 있다. 생성기를 다시 돌리기 전까지 데이터가 사라지지 않는다.

이것은 이 저장소의 기존 판단과 같은 방향이다 — 빈 문자열과 0 을 `*OK` 로 가른 것도,
모르는 것을 아는 척하지 않으려는 것이었다.

## 8. 조건검색 — `ka10173` 만 손으로 쓴다

6개(`ka10171`·`ka10172`·`ka10174` 와 미국 짝)는 같은 소켓 위의 요청/응답이라 REST 처럼
생성한다. 패키지는 `domestic/condition`·`overseas/condition`.

`ka10173` **하나만 예외**다. 응답이 **두 벌**이다 — 조회 결과가 한 번 오고, 그 뒤로 `REAL`
푸시가 계속 온다. 스펙에서도 이 API 만 `is_section` 으로 본문이 갈라진다
(`조회 데이터` / `실시간 데이터`).

### `usa20290` — 푸시는 오지만 스키마가 문서에 없다 (확인 완료)

미국 짝인 `usa20290` 은 응답이 **조회 한 벌뿐**이다. `is_section` 도 없고 `REAL` 푸시 절도
없다. `ka10173` 에 있는 `type`·`name`·`values`·FID 가 **통째로 빠져 있다**(대신 `stexTp` 가
하나 더 있다).

공식 예제 `request_overseas_realtime_condition_search_async.py` 가 이걸 정리해 준다:

- **푸시는 실제로 온다** — 예제가 `collect_realtime(..., max_messages=10)` 으로 실시간을 모은다.
- **그런데 FID 표가 비어 있다** — `COLUMNS: dict[str, str] = {}`. 스펙에 행이 없으니 채울 수 없다.
- 예제 머리에 `수동 생성 필요` 마커가 붙어 있다. **키움 쪽도 이 API 를 예외로 취급한다.**
- `trnm` 은 `GCNSRREQ` 다(§2 의 문서 불일치는 이쪽이 맞다).

그래서 이렇게 낸다: 조회 응답은 `ka10173` 처럼 **타입을 붙여** 내고, 푸시는 **`Raw` 맵으로만**
흘린다. FID 이름을 지어내지 않는다 — 스펙에 없는 것을 있는 것처럼 만들지 않는다. §7 의
`Raw` 가 정확히 이런 자리를 위한 것이다.

doc 주석에 **왜 이것만 맵인지** 적는다. "지원하지 않는다"가 아니라 "이름표가 없다"가 사실이고,
둘은 다르다.

`is_section` 은 **스펙 전체에서 2행뿐이고 둘 다 `ka10173` 안**이다. 2단계에 생성된 304개에는
하나도 없다 — 즉 기존 생성물에 새어 든 것이 없다.

이 둘은 **요청/응답과 채널을 함께 돌려주는 손으로 쓴 메서드**로 낸다:

```go
res, ch, err := c.DomesticCondition.RequestDomesticRealtimeConditionSearch(ctx, req)
```

**생성기에 이 한 경우를 위한 분기를 넣지 않는다.** 337개 중 2개를 위해 생성기를 복잡하게
만드는 것보다, 2개를 손으로 쓰고 생성기를 단순하게 두는 편이 낫다. 손으로 쓴 파일은
생성물 헤더가 없으므로 `cleanGenerated` 가 건드리지 않는다(2단계에서 이미 그렇게 동작한다).

## 9. 생성기 확장

`Group` 에 `Kind` 를 더한다 — `rest` · `realtime` · `condition`. `RenderSubClient` 가 `Kind` 로
템플릿을 고른다. 실시간 하위 클라이언트는 `*transport.Client` 대신 WS 클라이언트를 받는다.

`Groups` 에 4줄이 는다:

| 메뉴 | market | pkg | 필드 | Kind |
|---|---|---|---|---|
| 국내주식 > 실시간시세 | domestic | realtime | `DomesticRealtime` | realtime |
| 국내주식 > 조건검색 | domestic | condition | `DomesticCondition` | condition |
| 미국주식 > 실시간시세 | overseas | realtime | `OverseasRealtime` | realtime |
| 미국주식 > 조건검색 | overseas | condition | `OverseasCondition` | condition |

같은 4줄이 `skipped` 에서 빠진다. `Clients` 는 25 → **29** 필드가 되고, 2단계에 넣어 둔
개수 계약 테스트(`TestClients_그룹_개수`)가 그 변경을 붙잡는다 — 숫자를 고치는 행위 자체가
"정말 늘릴 생각이었나"를 한 번 묻는다.

`newClients` 는 전송 둘(`*transport.Client`, WS 클라이언트)을 받는다. 생성물이므로
손으로 고칠 것이 없다.

## 10. 검증

- **골든 테스트 확장** — 2단계의 `tools/gen/golden_test.go` 가 그대로 새 패키지를 덮는다.
  생성물과 스펙의 바이트 일치, 손으로 고친 생성물 금지가 자동으로 따라온다.
- **가짜 WS 서버** — `httptest` 위에 세운다. 실서버 없이 다음을 전부 검증한다:
  핸드셰이크, `REG`/`REMOVE` 왕복, PING 응답, **끊고 다시 붙였을 때 재구독이 실제로
  나가는지**, `*GapError` 가 채널에 흐르는지, 느린 소비자가 소켓 전체를 막지 않는지,
  `ctx` 취소가 `REMOVE` 를 보내고 채널을 닫는지.
  이것들이 3단계에서 제일 깨지기 쉬운 부분이고, 전부 실서버 없이 잡을 수 있다.
- **FID 표 완전성 테스트** — 스펙의 FID 400개가 표에 **전부** 있는지.
  `groups.go` 의 카테고리 완전성 테스트와 같은 성격이다.
- **외부 모듈 확인** — 1·2단계와 같이, `internal` 타입이 시그니처에 새어 채널을 외부에서
  못 쓰게 되는 일이 없는지 별도 모듈에서 빌드해 본다.

## 11. 위험

| 위험 | 다루는 법 |
|---|---|
| ~~핸드셰이크 미확인~~ | **해소됨** — 공식 `ws_client.py` 에서 확인(§3). 근거를 `SOURCE.md` 에 기록 |
| ~~`values` 가 맵인지 배열인지~~ | **해소됨** — 공식 `decoders.py` 가 맵으로 확정(§7) |
| **FID 400개 이름을 지어낸다** | 한글명을 모든 줄에 주석으로. 검토를 구현과 분리 |
| **모의투자가 실시간을 주는지 미확인** | 안 주면 가짜 서버 테스트가 유일한 그물이 된다. 그래서 가짜 서버를 얇게 만들지 않는다 |
| ~~미국 `trnm` 의 `G` 접두어 불일치~~ | **해소됨** — 공식 예제가 `GCNSRREQ` 로 확정(§8) |
| **`usa20290` 의 푸시 FID 가 스펙에 없다** | 푸시는 온다. 조회는 타입, 푸시는 `Raw` 맵으로만(§8) |
| **키움이 FID 를 추가하면 조용히 버려진다** | 값 구조체마다 `Raw` 맵을 둬 원본을 보존(§7) |

## 12. 커밋 분리

1. 확인한 WS 규약(핸드셰이크·PING·`values` 맵·REG 배열)을 `SOURCE.md` 에 근거와 함께 기록
2. `internal/wstransport` — 연결·핸드셰이크·PING·재연결·멀티플렉싱 (가짜 서버 테스트 포함)
3. `tools/gen/fids.go` — FID 표 400개 (완전성 테스트 포함)
4. 생성기 `Kind` 확장과 실시간 23벌 생성
5. 조건검색 7개 생성 + `ka10173` 손으로(+ `usa20290` 은 §8 결론에 따라)
6. README·검증
