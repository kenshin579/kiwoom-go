# 스펙 출처

- 저장소: https://github.com/Kiwoom-Securities/Kiwoom-REST-API
- 스펙: `kiwoom/_data/kiwoom_api_spec.json` → `kiwoom_api_spec.json`
- 이름표: `examples/**/*.py` 의 `# api_id:` 머리말과 파일명에서 추출 → `api_names.json`
- 받은 커밋: 234560d213acd8871ae344b5481aecd2f30287fa
- 받은 날짜: 2026-09-14

## 갱신하는 법

1. 같은 경로에서 스펙을 다시 받는다.
2. 저장소 tarball 을 받아 예제 머리말에서 이름표를 다시 뽑는다. 이때 파일명 끝의
   `_async` / `_pubsub` 접미사는 **떼고** 저장한다 — 이 둘은 같은 API 를 두 가지
   예제 코드 스타일(콜백 방식 vs pub/sub 방식)로 보여준 것일 뿐 API 이름이 아니라서,
   접미사를 남기면 3단계 코드 생성에서 잘못된 Go 메서드 이름이 나오고 순회 순서에
   따라 결과가 흔들린다(비결정적 벤더링). 접미사를 떼면 같은 api_id 의 두 파일이
   같은 이름으로 접혀 결정적이 된다. 접은 뒤에도 같은 api_id 가 서로 다른 이름
   두 개에 걸리면 스크립트가 실패해야 한다(조용히 하나를 고르면 안 됨).
3. 이 파일의 커밋 SHA·날짜를 고친다.
4. `cd tools && go run ./gen/main` 으로 코드를 다시 생성한다.

원격을 빌드 때 받아오지 않는 이유: 재현 가능해야 하고, 키움이 스펙을 바꾸면
**의도적인 갱신 커밋**으로 드러나는 편이 낫다.

## WebSocket 규약 (스펙에 없어 예제에서 확인함)

`kiwoom_api_spec.json` 은 api-id 별 메시지만 담는다. 연결을 여는 절차는 없다 —
`LOGIN`·`PING`·`PONG` 이 스펙 전체에서 **0회**다.

아래는 같은 커밋(`234560d213acd8871ae344b5481aecd2f30287fa`)의 공식 Python 클라이언트에서
읽은 것이다. **코드를 옮겨 오지 않았다. 규약만 적었다.**

| 사실 | 출처 |
|---|---|
| 로그인 패킷 `{"trnm":"LOGIN","token":…}` | `kiwoom/core/ws_client.py` `_login` |
| 로그인 응답을 먼저 받아야 하고 `return_code != 0` 이면 실패 | 같은 파일 `_await_login_ack` |
| PING 은 **받은 것을 그대로 echo** | 같은 파일 `_receive_non_ping_message` |
| 인증 실패 시 토큰 재발급 후 **로그인만** 재시도 | 같은 파일 `connect(retry_on_auth_failure=True)` |
| REG 패킷의 `item`·`type` 이 **배열** | `examples/국내주식/실시간시세/subscribe_domestic_stock_trade_async.py` |
| REAL 푸시의 `item` 은 **스칼라**, `values` 는 **맵** | `kiwoom/realtime/decoders.py` 머리말 |
| `usa20290` 은 푸시가 오지만 FID 표가 비어 있음(`COLUMNS = {}`) | `examples/미국주식/조건검색/request_overseas_realtime_condition_search_async.py` |
| 미국 조건검색 `trnm` 은 `GCNSRREQ` | 같은 파일 |

메시지 모양:

```json
// 등록
{"trnm":"REG","grp_no":"1","refresh":"1","data":[{"item":["005930"],"type":["0B"]}]}

// 푸시
{"trnm":"REAL","data":[{"type":"0B","name":"주식체결","item":"005930",
                        "values":{"10":"-82000","20":"093015"}}]}
```

스펙과 **어긋나는** 곳(이 표가 이긴다):

- 스펙은 REG 의 `item`·`type` 을 depth-1 `String` 으로 적었지만 실제로는 배열이다.
- 스펙은 `values` 를 실시간 23곳에서 `LIST`, `ka10173` 에서 `Object` 로 적었다.
  실제는 **맵**이다. `LIST` 쪽이 문서 오류다.

규약을 다시 확인하려면 위 파일들을 같은 커밋에서 읽어라. 원격을 빌드 때 받아오지 않는
이유는 스펙과 같다 — 재현 가능해야 하고, 바뀌면 의도적인 갱신 커밋으로 드러나야 한다.
