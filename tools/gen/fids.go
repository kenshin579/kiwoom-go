package gen

// Fid 는 실시간 값 필드 하나다.
type Fid struct {
	FID    string // 스펙의 element. 숫자다
	Name   string // Go 필드 이름
	Korean string // 스펙의 한글명. 사람이 표를 훑을 수 있게 남긴다
}

// Fids 는 FID → Go 필드 이름 표다.
//
// **여기는 지어낸 이름이다.** groups.go 와 같은 이유로 손으로 적는다 — 스펙이 주는 것은
// FID 숫자와 한글명뿐이고, 영문명은 어디에도 없다. GoName 에 맡기면 "N10"·"N9001" 이 되어
// 공개 라이브러리 표면으로 쓸 수 없다.
//
// 한글명을 모든 줄에 남기는 이유: 틀린 이름은 표를 훑으면 사람 눈에 보인다.
// 이름을 고칠 때는 **생성물을 고치지 말고** 이 표를 고치고 다시 생성하라.
//
// 한글명이 갈리는 FID 가 67개 있다. 전부 표기 차이다(`매도1호가` vs `매도호가1`,
// `시간` vs `체결시간`). 의미가 실제로 충돌하는 FID 는 없어서 하나로 접었고,
// 버린 표기는 그 줄 주석에 남겼다.
//
// 반대로 **한글명이 같은데 FID 가 다른** 것이 여덟 쌍 있다(예상체결가 23·291,
// 매수비율 129·1032, 거래소구분 337·2134·9081, 대출일 916·923, 신용구분 917·922,
// Extra Item 924·951·1279, 시간 20·21, 체결량 15·911). 이름을 하나로 접을 수 없어 —
// 23 과 291 은 같은 실시간 타입(0D)에 **함께** 들어간다 — 어느 쪽이 무엇인지 그 줄 주석에
// 적고 이름을 갈랐다.
//
// 단위 괄호는 이름에 녹인다. 다만 `시가총액(억)` 의 억은 10^8 원이라 Billion(10^9) 이
// 아니다 — MarketCapHundredMillionWon 으로 적는다.
//
// 미국주식 쪽 한글명에 붙은 꼬리표 "사용"(`세금 사용`·`수수료 사용` 등 8개)은 스펙 표의
// 주석이지 필드 이름의 일부가 아니라서 Go 이름에 넣지 않았다.
//
// 매도/매수는 한국 증권 관례대로 옮긴다 — **매도 = Ask/Sell, 매수 = Bid/Buy.**
// 호가창 쪽은 Ask/Bid, 체결·주문 쪽은 Sell/Buy 를 쓴다. 한 글자 차이로 정반대가 되는
// 자리라 축약도 하지 않는다.
//
// 슬라이스로 두는 이유: groups.go 와 같다 — 생성 순서가 맵 순회에 흔들리지 않아야 한다.
var Fids = []Fid{
	{"10", "CurrentPrice", "현재가"},
	{"11", "PrevDayDiff", "전일대비"},
	{"12", "ChangeRate", "등락율"},
	{"13", "CumulativeVolume", "누적거래량"},
	{"14", "CumulativeTradeAmount", "누적거래대금"},
	{"15", "TradeVolume", "거래량"}, // 표기 차이: 체결량
	{"16", "OpenPrice", "시가"},
	{"17", "HighPrice", "고가"},
	{"18", "LowPrice", "저가"},
	{"20", "TradeTime", "체결시간"}, // 표기 차이: 시간
	{"21", "QuoteTime", "호가시간"}, // 표기 차이: 시간
	{"22", "TradeDate", "체결일자"},
	{"23", "ExpectedPrice", "예상체결가"}, // 스펙이 "단위: 원" 만 적었다. FID 291 과 함께 호가잔량(0D) 에만 들어간다
	{"24", "ExpectedQuantity", "예상체결수량"},
	{"25", "PrevDayDiffSign", "전일대비기호"},
	{"26", "PrevDayVolumeDiff", "전일거래량대비"}, // 표기 차이: 전일거래량대비(계약,주)
	{"27", "BestAskPrice", "(최우선)매도호가"},
	{"28", "BestBidPrice", "(최우선)매수호가"},
	{"29", "TradeAmountChange", "거래대금증감"},
	{"30", "PrevDayVolumeRatio", "전일거래량대비(비율)"},
	{"31", "TurnoverRate", "거래회전율"},
	{"32", "TradeCost", "거래비용"},
	{"36", "NAV", "NAV"},
	{"37", "NAVPrevDayDiff", "NAV전일대비"},
	{"38", "NAVChangeRate", "NAV등락율"},
	{"39", "TrackingErrorRate", "추적오차율"},
	{"41", "AskPrice1", "매도1호가"},        // 표기 차이: 매도호가1
	{"42", "AskPrice2", "매도2호가"},        // 표기 차이: 매도호가2
	{"43", "AskPrice3", "매도3호가"},        // 표기 차이: 매도호가3
	{"44", "AskPrice4", "매도4호가"},        // 표기 차이: 매도호가4
	{"45", "AskPrice5", "매도5호가"},        // 표기 차이: 매도호가5
	{"46", "AskPrice6", "매도6호가"},        // 표기 차이: 매도호가6
	{"47", "AskPrice7", "매도7호가"},        // 표기 차이: 매도호가7
	{"48", "AskPrice8", "매도8호가"},        // 표기 차이: 매도호가8
	{"49", "AskPrice9", "매도9호가"},        // 표기 차이: 매도호가9
	{"50", "AskPrice10", "매도10호가"},      // 표기 차이: 매도호가10
	{"51", "BidPrice1", "매수1호가"},        // 표기 차이: 매수호가1
	{"52", "BidPrice2", "매수2호가"},        // 표기 차이: 매수호가2
	{"53", "BidPrice3", "매수3호가"},        // 표기 차이: 매수호가3
	{"54", "BidPrice4", "매수4호가"},        // 표기 차이: 매수호가4
	{"55", "BidPrice5", "매수5호가"},        // 표기 차이: 매수호가5
	{"56", "BidPrice6", "매수6호가"},        // 표기 차이: 매수호가6
	{"57", "BidPrice7", "매수7호가"},        // 표기 차이: 매수호가7
	{"58", "BidPrice8", "매수8호가"},        // 표기 차이: 매수호가8
	{"59", "BidPrice9", "매수9호가"},        // 표기 차이: 매수호가9
	{"60", "BidPrice10", "매수10호가"},      // 표기 차이: 매수호가10
	{"61", "AskQuantity1", "매도1호가잔량"},   // 표기 차이: 매도호가수량1
	{"62", "AskQuantity2", "매도2호가잔량"},   // 표기 차이: 매도호가수량2
	{"63", "AskQuantity3", "매도3호가잔량"},   // 표기 차이: 매도호가수량3
	{"64", "AskQuantity4", "매도4호가잔량"},   // 표기 차이: 매도호가수량4
	{"65", "AskQuantity5", "매도5호가잔량"},   // 표기 차이: 매도호가수량5
	{"66", "AskQuantity6", "매도6호가잔량"},   // 표기 차이: 매도호가수량6
	{"67", "AskQuantity7", "매도7호가잔량"},   // 표기 차이: 매도호가수량7
	{"68", "AskQuantity8", "매도8호가잔량"},   // 표기 차이: 매도호가수량8
	{"69", "AskQuantity9", "매도9호가잔량"},   // 표기 차이: 매도호가수량9
	{"70", "AskQuantity10", "매도10호가잔량"}, // 표기 차이: 매도호가수량10
	{"71", "BidQuantity1", "매수1호가잔량"},   // 표기 차이: 매수호가수량1
	{"72", "BidQuantity2", "매수2호가잔량"},   // 표기 차이: 매수호가수량2
	{"73", "BidQuantity3", "매수3호가잔량"},   // 표기 차이: 매수호가수량3
	{"74", "BidQuantity4", "매수4호가잔량"},   // 표기 차이: 매수호가수량4
	{"75", "BidQuantity5", "매수5호가잔량"},   // 표기 차이: 매수호가수량5
	{"76", "BidQuantity6", "매수6호가잔량"},   // 표기 차이: 매수호가수량6
	{"77", "BidQuantity7", "매수7호가잔량"},   // 표기 차이: 매수호가수량7
	{"78", "BidQuantity8", "매수8호가잔량"},   // 표기 차이: 매수호가수량8
	{"79", "BidQuantity9", "매수9호가잔량"},   // 표기 차이: 매수호가수량9
	{"80", "BidQuantity10", "매수10호가잔량"}, // 표기 차이: 매수호가수량10
	// 81~100 은 **가격인지 잔량인지 스펙이 말하지 않는다.** 설명 칸이 비어 있다.
	// 이름에서 Price 를 뺀 이유: 같은 "직전대비" 꼬리를 단 이웃이 전부 잔량의 증감이다 —
	// 0E 의 132·136(시간외 총잔량직전대비)은 설명이 "단위: 1주, 부호가 포함된 숫자" 이고,
	// 0D 의 122·126 은 한글명 자체가 "총잔량직전대비" 다. 0D 에 per-level 잔량 증감 FID 가
	// 따로 없다는 점도 같은 쪽을 가리킨다. 그래도 한글명은 "호가직전대비" 라 단정할 수 없어
	// Quantity 도 붙이지 않았다. 실데이터로 확인되면 그때 이름을 확정하라.
	{"81", "AskPrevDiff1", "매도1호가직전대비"},    // 표기 차이: 매도호가직전대비1
	{"82", "AskPrevDiff2", "매도2호가직전대비"},    // 표기 차이: 매도호가직전대비2
	{"83", "AskPrevDiff3", "매도3호가직전대비"},    // 표기 차이: 매도호가직전대비3
	{"84", "AskPrevDiff4", "매도4호가직전대비"},    // 표기 차이: 매도호가직전대비4
	{"85", "AskPrevDiff5", "매도5호가직전대비"},    // 표기 차이: 매도호가직전대비5
	{"86", "AskPrevDiff6", "매도6호가직전대비"},    // 표기 차이: 매도호가직전대비6
	{"87", "AskPrevDiff7", "매도7호가직전대비"},    // 표기 차이: 매도호가직전대비7
	{"88", "AskPrevDiff8", "매도8호가직전대비"},    // 표기 차이: 매도호가직전대비8
	{"89", "AskPrevDiff9", "매도9호가직전대비"},    // 표기 차이: 매도호가직전대비9
	{"90", "AskPrevDiff10", "매도10호가직전대비"},  // 표기 차이: 매도호가직전대비10
	{"91", "BidPrevDiff1", "매수1호가직전대비"},    // 표기 차이: 매수호가직전대비1
	{"92", "BidPrevDiff2", "매수2호가직전대비"},    // 표기 차이: 매수호가직전대비2
	{"93", "BidPrevDiff3", "매수3호가직전대비"},    // 표기 차이: 매수호가직전대비3
	{"94", "BidPrevDiff4", "매수4호가직전대비"},    // 표기 차이: 매수호가직전대비4
	{"95", "BidPrevDiff5", "매수5호가직전대비"},    // 표기 차이: 매수호가직전대비5
	{"96", "BidPrevDiff6", "매수6호가직전대비"},    // 표기 차이: 매수호가직전대비6
	{"97", "BidPrevDiff7", "매수7호가직전대비"},    // 표기 차이: 매수호가직전대비7
	{"98", "BidPrevDiff8", "매수8호가직전대비"},    // 표기 차이: 매수호가직전대비8
	{"99", "BidPrevDiff9", "매수9호가직전대비"},    // 표기 차이: 매수호가직전대비9
	{"100", "BidPrevDiff10", "매수10호가직전대비"}, // 표기 차이: 매수호가직전대비10
	{"121", "TotalAskQuantity", "매도호가총잔량"},
	{"122", "TotalAskQuantityPrevDiff", "매도호가총잔량직전대비"},
	{"125", "TotalBidQuantity", "매수호가총잔량"},
	{"126", "TotalBidQuantityPrevDiff", "매수호가총잔량직전대비"},
	{"128", "NetBidQuantity", "순매수잔량"},
	{"129", "BidRatio", "매수비율"}, // 호가잔량 기준. FID 1032 는 체결량 기준이다
	{"131", "AfterHoursTotalAskQuantity", "시간외매도호가총잔량"},
	{"132", "AfterHoursTotalAskQuantityPrevDiff", "시간외매도호가총잔량직전대비"},
	{"135", "AfterHoursTotalBidQuantity", "시간외매수호가총잔량"},
	{"136", "AfterHoursTotalBidQuantityPrevDiff", "시간외매수호가총잔량직전대비"},
	{"138", "NetAskQuantity", "순매도잔량"},
	{"139", "AskRatio", "매도비율"},
	{"141", "SellMember1", "매도거래원1"},
	{"142", "SellMember2", "매도거래원2"},
	{"143", "SellMember3", "매도거래원3"},
	{"144", "SellMember4", "매도거래원4"},
	{"145", "SellMember5", "매도거래원5"},
	{"146", "SellMemberCode1", "매도거래원코드1"},
	{"147", "SellMemberCode2", "매도거래원코드2"},
	{"148", "SellMemberCode3", "매도거래원코드3"},
	{"149", "SellMemberCode4", "매도거래원코드4"},
	{"150", "SellMemberCode5", "매도거래원코드5"},
	{"151", "BuyMember1", "매수거래원1"},
	{"152", "BuyMember2", "매수거래원2"},
	{"153", "BuyMember3", "매수거래원3"},
	{"154", "BuyMember4", "매수거래원4"},
	{"155", "BuyMember5", "매수거래원5"},
	{"156", "BuyMemberCode1", "매수거래원코드1"},
	{"157", "BuyMemberCode2", "매수거래원코드2"},
	{"158", "BuyMemberCode3", "매수거래원코드3"},
	{"159", "BuyMemberCode4", "매수거래원코드4"},
	{"160", "BuyMemberCode5", "매수거래원코드5"},
	{"161", "SellMemberQuantity1", "매도거래원수량1"},
	{"162", "SellMemberQuantity2", "매도거래원수량2"},
	{"163", "SellMemberQuantity3", "매도거래원수량3"},
	{"164", "SellMemberQuantity4", "매도거래원수량4"},
	{"165", "SellMemberQuantity5", "매도거래원수량5"},
	{"166", "SellMemberChange1", "매도거래원별증감1"},
	{"167", "SellMemberChange2", "매도거래원별증감2"},
	{"168", "SellMemberChange3", "매도거래원별증감3"},
	{"169", "SellMemberChange4", "매도거래원별증감4"},
	{"170", "SellMemberChange5", "매도거래원별증감5"},
	{"171", "BuyMemberQuantity1", "매수거래원수량1"},
	{"172", "BuyMemberQuantity2", "매수거래원수량2"},
	{"173", "BuyMemberQuantity3", "매수거래원수량3"},
	{"174", "BuyMemberQuantity4", "매수거래원수량4"},
	{"175", "BuyMemberQuantity5", "매수거래원수량5"},
	{"176", "BuyMemberChange1", "매수거래원별증감1"},
	{"177", "BuyMemberChange2", "매수거래원별증감2"},
	{"178", "BuyMemberChange3", "매수거래원별증감3"},
	{"179", "BuyMemberChange4", "매수거래원별증감4"},
	{"180", "BuyMemberChange5", "매수거래원별증감5"},
	{"200", "ExpectedPricePrevCloseDiff", "예상체결가전일종가대비"},
	{"201", "ExpectedPricePrevCloseChangeRate", "예상체결가전일종가대비등락율"},
	{"202", "SellQuantity", "매도수량"},
	{"204", "SellAmount", "매도금액"},
	{"206", "BuyQuantity", "매수수량"},
	{"208", "BuyAmount", "매수금액"},
	{"210", "NetBuyQuantity", "순매수수량"},
	{"211", "NetBuyQuantityChange", "순매수수량증감"},
	{"212", "NetBuyAmount", "순매수금액"},
	{"213", "NetBuyAmountChange", "순매수금액증감"},
	{"214", "MarketOpenRemainingTime", "장시작예상잔여시간"},
	{"215", "MarketOperationType", "장운영구분"},
	{"216", "InvestorTicker", "투자자별ticker"},
	{"228", "TradeStrength", "체결강도"},
	{"238", "ExpectedPricePrevCloseDiffSign", "예상체결가전일종가대비기호"},
	{"251", "UpperLimitCount", "상한종목수"},
	{"252", "AdvancingCount", "상승종목수"},
	{"253", "UnchangedCount", "보합종목수"},
	{"254", "LowerLimitCount", "하한종목수"},
	{"255", "DecliningCount", "하락종목수"},
	{"256", "TradedStockCount", "거래형성종목수"},
	{"257", "TradedStockRatio", "거래형성비율"},
	{"261", "ForeignSellEstimateTotal", "외국계매도추정합"},
	{"262", "ForeignSellEstimateTotalChange", "외국계매도추정합변동"},
	{"263", "ForeignBuyEstimateTotal", "외국계매수추정합"},
	{"264", "ForeignBuyEstimateTotalChange", "외국계매수추정합변동"},
	{"265", "NAVIndexDivergenceRate", "NAV/지수괴리율"},
	{"266", "NAVETFDivergenceRate", "NAV/ETF괴리율"},
	{"267", "ForeignNetBuyEstimateTotal", "외국계순매수추정합"},
	{"268", "ForeignNetBuyChange", "외국계순매수변동"},
	{"271", "SellMemberColor1", "매도거래원색깔1"},
	{"272", "SellMemberColor2", "매도거래원색깔2"},
	{"273", "SellMemberColor3", "매도거래원색깔3"},
	{"274", "SellMemberColor4", "매도거래원색깔4"},
	{"275", "SellMemberColor5", "매도거래원색깔5"},
	{"281", "BuyMemberColor1", "매수거래원색깔1"},
	{"282", "BuyMemberColor2", "매수거래원색깔2"},
	{"283", "BuyMemberColor3", "매수거래원색깔3"},
	{"284", "BuyMemberColor4", "매수거래원색깔4"},
	{"285", "BuyMemberColor5", "매수거래원색깔5"},
	{"290", "SessionType", "장구분"},
	{"291", "ExpectedTradePrice", "예상체결가"}, // 스펙이 "예상체결 시간동안에만 유효한 값" 이라 적었다. 23 과 함께 호가잔량(0D) 에만 들어간다 — 예상체결(0H) 에는 둘 다 없다
	{"292", "ExpectedTradeVolume", "예상체결량"},
	{"293", "ExpectedTradePrevDayDiffSign", "예상체결가전일대비기호"},
	{"294", "ExpectedTradePrevDayDiff", "예상체결가전일대비"},
	{"295", "ExpectedTradePrevDayChangeRate", "예상체결가전일대비등락율"},
	{"297", "RandomExtension", "임의연장"},
	{"299", "PrevDayVolumeExpectedTradeRate", "전일거래량대비예상체결율"},
	{"302", "StockName", "종목명"},
	{"305", "UpperLimitPrice", "상한가"},
	{"306", "LowerLimitPrice", "하한가"},
	{"307", "BasePrice", "기준가"},
	{"311", "MarketCapHundredMillionWon", "시가총액(억)"}, // 억원 단위다 — 10^8 원. Billion(10^9) 이 아니다
	{"337", "ExchangeType", "거래소구분"},                 // 거래원(0F)의 거래소구분. FID 2134·9081 과 한글명이 같아 이름을 갈랐다
	{"370", "StockInfo", "종목정보"},
	{"382", "MarginRateDisplay", "증거금율표시"},
	{"567", "UpperLimitTime", "상한가발생시간"},
	{"568", "LowerLimitTime", "하한가발생시간"},
	{"592", "PreMarketRandomExtension", "장전임의연장"},
	{"593", "PostMarketRandomExtension", "장후임의연장"},
	{"594", "CurrencyUnit", "통화단위"},
	{"620", "TodayAveragePrice", "당일거래평균가"},
	{"621", "LPAskQuantity1", "LP매도호가수량1"},
	{"622", "LPAskQuantity2", "LP매도호가수량2"},
	{"623", "LPAskQuantity3", "LP매도호가수량3"},
	{"624", "LPAskQuantity4", "LP매도호가수량4"},
	{"625", "LPAskQuantity5", "LP매도호가수량5"},
	{"626", "LPAskQuantity6", "LP매도호가수량6"},
	{"627", "LPAskQuantity7", "LP매도호가수량7"},
	{"628", "LPAskQuantity8", "LP매도호가수량8"},
	{"629", "LPAskQuantity9", "LP매도호가수량9"},
	{"630", "LPAskQuantity10", "LP매도호가수량10"},
	{"631", "LPBidQuantity1", "LP매수호가수량1"},
	{"632", "LPBidQuantity2", "LP매수호가수량2"},
	{"633", "LPBidQuantity3", "LP매수호가수량3"},
	{"634", "LPBidQuantity4", "LP매수호가수량4"},
	{"635", "LPBidQuantity5", "LP매수호가수량5"},
	{"636", "LPBidQuantity6", "LP매수호가수량6"},
	{"637", "LPBidQuantity7", "LP매수호가수량7"},
	{"638", "LPBidQuantity8", "LP매수호가수량8"},
	{"639", "LPBidQuantity9", "LP매수호가수량9"},
	{"640", "LPBidQuantity10", "LP매수호가수량10"},
	{"666", "ELWParity", "ELW패리티"},
	{"667", "ELWGearingRatio", "ELW기어링비율"},
	{"668", "ELWBreakEvenRate", "ELW손익분기율"},
	{"669", "ELWCapitalSupportPoint", "ELW자본지지점"},
	{"670", "ELWTheoreticalPrice", "ELW이론가"},
	{"671", "ELWImpliedVolatility", "ELW내재변동성"},
	{"672", "ELWDelta", "ELW델타"},
	{"673", "ELWGamma", "ELW감마"},
	{"674", "ELWTheta", "ELW쎄타"},
	{"675", "ELWVega", "ELW베가"},
	{"676", "ELWRho", "ELW로"},
	{"689", "EarlyTerminationELW", "조기종료ELW발생"},
	{"691", "KnockOutProximity", "K.O 접근도"},
	{"706", "LPQuoteImpliedVolatility", "LP호가내재변동성"},
	{"732", "CFDTradeCost", "CFD거래비용"},
	{"841", "SequenceNumber", "일련번호"},
	{"843", "InsertDeleteType", "삽입삭제 구분"},
	{"851", "PrevDaySameTimeVolumeRatio", "전일 동시간 거래량 비율"},
	{"852", "StockLoanTradeCost", "대주거래비용"},
	{"900", "OrderQuantity", "주문수량"},
	{"901", "OrderPrice", "주문가격"},
	{"902", "UnfilledQuantity", "미체결수량"},
	{"903", "CumulativeExecutedAmount", "체결누계금액"},
	{"904", "OriginalOrderNumber", "원주문번호"},
	{"905", "OrderType", "주문구분"},
	{"906", "OrderPriceType", "매매구분"}, // 방향이 아니라 주문 호가 유형이다 — 스펙 값이 보통·시장가·조건부지정가·IOC·FOK. 907 TradeSide 와 헷갈리면 안 된다
	{"907", "TradeSide", "매도/수 구분"},   // 표기 차이: 매도수구분. 01·02 또는 1·2 로 오는 매도/매수 구분 코드
	{"908", "OrderExecutionTime", "주문/체결시간"},
	{"909", "ExecutionNumber", "체결번호"},
	{"910", "ExecutionPrice", "체결가"},
	{"911", "ExecutionQuantity", "체결량"},
	{"912", "OrderBusinessType", "주문업무분류"},
	{"913", "OrderStatus", "주문상태"},
	{"914", "UnitExecutionPrice", "단위체결가"},
	{"915", "UnitExecutionQuantity", "단위체결량"},
	{"916", "LoanDate", "대출일"},    // 잔고(04)의 대출일. FID 923 은 체결용이다
	{"917", "CreditType", "신용구분"}, // 잔고(04)의 신용구분. FID 922 는 체결용이다
	{"918", "ExpiryDate", "만기일"},
	{"919", "RejectReason", "거부사유"},
	{"920", "ScreenNumber", "화면번호"},
	{"921", "TerminalNumber", "터미널번호"},
	{"922", "ExecutionCreditType", "신용구분"}, // 주문체결(00)의 신용구분 — 스펙이 "실시간 체결용" 이라 적었다
	{"923", "ExecutionLoanDate", "대출일"},    // 주문체결(00)의 대출일 — 스펙이 "실시간 체결용" 이라 적었다
	{"924", "ExtraItem924", "Extra Item"},  // 스펙이 이름을 주지 않았다. FID 를 이름에 남긴다
	{"930", "HoldingQuantity", "보유수량"},
	{"931", "PurchasePrice", "매입단가"},
	{"932", "TotalPurchaseAmountTodayCumulative", "총매입가(당일누적)"},
	{"933", "OrderableQuantity", "주문가능수량"},
	{"934", "TodaySellQuantity", "당일매도수량 사용"},
	{"936", "TodayBuyQuantity", "당일매수수량 사용"},
	{"938", "TodayTradingFee", "당일매매수수료"},
	{"939", "TodayTradingTax", "당일매매세금"},
	{"945", "TodayNetBuyQuantity", "당일순매수량"},
	{"946", "BalanceTradeSide", "매도/매수구분"}, // 잔고(04)의 매도/매수 구분. FID 907 과 다른 필드다
	{"950", "TodayTotalSellProfit", "당일총매도손익"},
	{"951", "ExtraItem951", "Extra Item"}, // 스펙이 이름을 주지 않았다. FID 를 이름에 남긴다
	{"957", "CreditAmount", "신용금액"},
	{"958", "CreditInterest", "신용이자"},
	{"959", "CollateralLoanQuantity", "담보대출수량"},
	{"990", "TodayRealizedProfitSecurities", "당일실현손익(유가)"},
	{"991", "TodayRealizedProfitRateSecurities", "당일실현손익율(유가)"},
	{"992", "TodayRealizedProfitCredit", "당일실현손익(신용)"},
	{"993", "TodayRealizedProfitRateCredit", "당일실현손익율(신용)"},
	{"1030", "SellExecutionVolume", "매도체결량"},
	{"1031", "BuyExecutionVolume", "매수체결량"},
	{"1032", "BuyRatio", "매수비율"}, // 체결량 기준. FID 129 는 호가잔량 기준이다
	{"1071", "SellExecutionCount", "매도체결건수"},
	{"1072", "BuyExecutionCount", "매수체결건수"},
	{"1091", "CountryName", "국가명"},
	{"1211", "ELWPremium", "ELW프리미엄"},
	{"1221", "VITriggerPrice", "VI발동가격"},
	{"1223", "TradeExecutionProcessTime", "매매체결처리시각"},
	{"1224", "VIReleaseTime", "VI해제시각"},
	{"1225", "VIApplyType", "VI적용구분"},
	{"1236", "StaticBasePrice", "기준가격 정적"},
	{"1237", "DynamicBasePrice", "기준가격 동적"},
	{"1238", "StaticDivergenceRate", "괴리율 정적"},
	{"1239", "DynamicDivergenceRate", "괴리율 동적"},
	{"1279", "ExtraItem1279", "Extra Item"}, // 스펙이 이름을 주지 않았다. FID 를 이름에 남긴다
	{"1313", "InstantTradeAmount", "순간거래대금"},
	{"1314", "NetBuyExecutionVolume", "순매수체결량"},
	{"1315", "SingleSellExecutionVolume", "매도체결량_단건"},
	{"1316", "SingleBuyExecutionVolume", "매수체결량_단건"},
	{"1489", "VITriggerPriceChangeRate", "VI발동가 등락율"},
	{"1490", "VITriggerCount", "VI발동횟수"},
	{"1497", "CFDMargin", "CFD증거금"},
	{"1498", "MaintenanceMargin", "유지증거금"},
	{"1890", "OpenTime", "시가시간"},
	{"1891", "HighTime", "고가시간"},
	{"1892", "LowTime", "저가시간"},
	{"2134", "OrderExchangeType", "거래소구분"}, // 주문체결(00)의 거래소구분. 0:통합, 1:KRX, 2:NXT
	{"2135", "ExchangeTypeName", "거래소구분명"},
	{"2136", "SORFlag", "SOR여부"},
	{"6044", "KRXAskQuantity1", "KRX 매도호가잔량1"},
	{"6045", "KRXAskQuantity2", "KRX 매도호가잔량2"},
	{"6046", "KRXAskQuantity3", "KRX 매도호가잔량3"},
	{"6047", "KRXAskQuantity4", "KRX 매도호가잔량4"},
	{"6048", "KRXAskQuantity5", "KRX 매도호가잔량5"},
	{"6049", "KRXAskQuantity6", "KRX 매도호가잔량6"},
	{"6050", "KRXAskQuantity7", "KRX 매도호가잔량7"},
	{"6051", "KRXAskQuantity8", "KRX 매도호가잔량8"},
	{"6052", "KRXAskQuantity9", "KRX 매도호가잔량9"},
	{"6053", "KRXAskQuantity10", "KRX 매도호가잔량10"},
	{"6054", "KRXBidQuantity1", "KRX 매수호가잔량1"},
	{"6055", "KRXBidQuantity2", "KRX 매수호가잔량2"},
	{"6056", "KRXBidQuantity3", "KRX 매수호가잔량3"},
	{"6057", "KRXBidQuantity4", "KRX 매수호가잔량4"},
	{"6058", "KRXBidQuantity5", "KRX 매수호가잔량5"},
	{"6059", "KRXBidQuantity6", "KRX 매수호가잔량6"},
	{"6060", "KRXBidQuantity7", "KRX 매수호가잔량7"},
	{"6061", "KRXBidQuantity8", "KRX 매수호가잔량8"},
	{"6062", "KRXBidQuantity9", "KRX 매수호가잔량9"},
	{"6063", "KRXBidQuantity10", "KRX 매수호가잔량10"},
	{"6064", "KRXTotalAskQuantity", "KRX 매도호가총잔량"},
	{"6065", "KRXTotalBidQuantity", "KRX 매수호가총잔량"},
	{"6066", "NXTAskQuantity1", "NXT 매도호가잔량1"},
	{"6067", "NXTAskQuantity2", "NXT 매도호가잔량2"},
	{"6068", "NXTAskQuantity3", "NXT 매도호가잔량3"},
	{"6069", "NXTAskQuantity4", "NXT 매도호가잔량4"},
	{"6070", "NXTAskQuantity5", "NXT 매도호가잔량5"},
	{"6071", "NXTAskQuantity6", "NXT 매도호가잔량6"},
	{"6072", "NXTAskQuantity7", "NXT 매도호가잔량7"},
	{"6073", "NXTAskQuantity8", "NXT 매도호가잔량8"},
	{"6074", "NXTAskQuantity9", "NXT 매도호가잔량9"},
	{"6075", "NXTAskQuantity10", "NXT 매도호가잔량10"},
	{"6076", "NXTBidQuantity1", "NXT 매수호가잔량1"},
	{"6077", "NXTBidQuantity2", "NXT 매수호가잔량2"},
	{"6078", "NXTBidQuantity3", "NXT 매수호가잔량3"},
	{"6079", "NXTBidQuantity4", "NXT 매수호가잔량4"},
	{"6080", "NXTBidQuantity5", "NXT 매수호가잔량5"},
	{"6081", "NXTBidQuantity6", "NXT 매수호가잔량6"},
	{"6082", "NXTBidQuantity7", "NXT 매수호가잔량7"},
	{"6083", "NXTBidQuantity8", "NXT 매수호가잔량8"},
	{"6084", "NXTBidQuantity9", "NXT 매수호가잔량9"},
	{"6085", "NXTBidQuantity10", "NXT 매수호가잔량10"},
	{"6086", "NXTTotalAskQuantity", "NXT 매도호가총잔량"},
	{"6087", "NXTTotalBidQuantity", "NXT 매수호가총잔량"},
	{"6100", "KRXMidPriceTotalAskQuantityChange", "KRX 중간가 매도 총잔량 증감"},
	{"6101", "KRXMidPriceTotalAskQuantity", "KRX 중간가 매도 총잔량"},
	{"6102", "KRXMidPrice", "KRX 중간가"},
	{"6103", "KRXMidPriceTotalBidQuantity", "KRX 중간가 매수 총잔량"},
	{"6104", "KRXMidPriceTotalBidQuantityChange", "KRX 중간가 매수 총잔량 증감"},
	{"6105", "NXTMidPriceTotalAskQuantityChange", "NXT중간가 매도 총잔량 증감"},
	{"6106", "NXTMidPriceTotalAskQuantity", "NXT중간가 매도 총잔량"},
	{"6107", "NXTMidPrice", "NXT중간가"},
	{"6108", "NXTMidPriceTotalBidQuantity", "NXT중간가 매수 총잔량"},
	{"6109", "NXTMidPriceTotalBidQuantityChange", "NXT중간가 매수 총잔량 증감"},
	{"6110", "KRXMidPriceDiff", "KRX중간가대비"},
	{"6111", "KRXMidPriceDiffSign", "KRX중간가대비 기호"},
	{"6112", "KRXMidPriceChangeRate", "KRX중간가대비등락율"},
	{"6113", "NXTMidPriceDiff", "NXT중간가대비"},
	{"6114", "NXTMidPriceDiffSign", "NXT중간가대비 기호"},
	{"6115", "NXTMidPriceChangeRate", "NXT중간가대비등락율"},
	{"8004", "PrevDaySellQuantity", "전일매도수량"},
	{"8005", "PrevDayBuyQuantity", "전일매수수량"},
	{"8018", "ProfitAmount", "손익금액"},
	{"8019", "RealizedProfitRate", "손익률(실현손익)"}, // 표기 차이: 손익율
	{"8043", "CurrencyCode", "통화코드"},
	{"8046", "ExchangeCode", "거래소코드"},
	{"8075", "Tax", "세금 사용"},
	{"9001", "StockOrSectorCode", "종목,업종코드"}, // 표기 차이: 종목코드, 종목코드,업종코드
	{"9008", "MarketType", "KOSPI,KOSDAQ,전체구분"},
	{"9068", "VITriggerType", "VI발동구분"},
	{"9069", "TriggerDirectionType", "발동방향구분"},
	{"9075", "PreMarketType", "장전구분"},
	{"9081", "TradeExchangeType", "거래소구분"}, // 체결(0B)의 거래소구분
	{"9201", "AccountNumber", "계좌번호"},
	{"9203", "OrderNumber", "주문번호"},
	{"9205", "ManagerEmployeeNumber", "관리자사번"},
	{"10010", "AfterHoursSinglePriceCurrentPrice", "시간외단일가_현재가"},
	{"13006", "Fee", "수수료 사용"},
	{"50072", "TradeSideName", "매도수구분명"},
	{"50073", "OrderPriceTypeName", "매매구분명"}, // 906 의 텍스트값. 스펙: "텍스트값(지정가, 시장가 등...)"
	{"50724", "RealizedProfitPurchaseAmount", "실현손익매입금 사용"},
	{"50725", "CurrencyConvertedRealizedProfitPurchaseAmount", "환전실현손익매입금액 사용"}, // 환전 = 통화 환산. 거래소(Exchange) 가 아니다
	{"50810", "OrderStopPrice", "주문STOP가격"},
	{"50841", "ReservationType", "예약구분"},
	{"50844", "CurrencyConvertedRealizedProfitAmount", "환전실현손익금액 사용"}, // 환전 = 통화 환산. 거래소(Exchange) 가 아니다
	{"51020", "LocalTradeTime", "현지 체결시간"},
	{"55190", "FinanceCountryCode", "(재무)국가코드 사용"},
}

// fidByID 는 조회용 인덱스다.
var fidByID = func() map[string]Fid {
	m := make(map[string]Fid, len(Fids))
	for _, f := range Fids {
		m[f.FID] = f
	}
	return m
}()

// LookupFid 는 FID 로 Go 이름을 찾는다.
func LookupFid(fid string) (Fid, bool) {
	f, ok := fidByID[fid]
	return f, ok
}
