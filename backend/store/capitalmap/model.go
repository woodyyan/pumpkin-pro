package capitalmap

import "time"

const (
	DefaultCacheTTL           = 30 * time.Second
	DefaultRefreshHintSeconds = 60
)

type Stock struct {
	Code             string   `json:"code"`
	Symbol           string   `json:"symbol"`
	Name             string   `json:"name"`
	Market           string   `json:"market"`
	Price            *float64 `json:"price"`
	PctChg           *float64 `json:"pctChg"`
	Amount           float64  `json:"-"`
	AmountYi         *float64 `json:"amountYi"`
	VolumeHands      *float64 `json:"volumeHands,omitempty"`
	TurnoverRate     *float64 `json:"turnoverRate"`
	PE               *float64 `json:"pe"`
	PETTM            *float64 `json:"peTtm,omitempty"`
	DynamicPE        *float64 `json:"dynamicPe,omitempty"`
	PESource         string   `json:"peSource"`
	PB               *float64 `json:"pb"`
	TotalMarketCap   *float64 `json:"-"`
	FloatMarketCap   *float64 `json:"-"`
	TotalMarketCapYi *float64 `json:"totalMarketCapYi"`
	FloatMarketCapYi *float64 `json:"floatMarketCapYi,omitempty"`
	MainNetInflow    *float64 `json:"-"`
	MainNetInflowYi  *float64 `json:"mainNetInflowYi"`
	Change60D        *float64 `json:"change60d,omitempty"`
	ChangeYTD        *float64 `json:"changeYtd,omitempty"`
}

type Sector struct {
	Code               string   `json:"code"`
	Name               string   `json:"name"`
	PctChg             *float64 `json:"pctChg"`
	Amount             float64  `json:"-"`
	AmountYi           *float64 `json:"amountYi"`
	AmountRatio        *float64 `json:"amountRatio,omitempty"`
	MainNetInflow      float64  `json:"-"`
	MainNetInflowYi    *float64 `json:"mainNetInflowYi"`
	NetInflowIntensity *float64 `json:"netInflowIntensity"`
	LeaderName         string   `json:"leaderName"`
	LeaderCode         string   `json:"leaderCode"`
}

type SnapshotResult struct {
	Stocks         []Stock
	TotalAvailable int
	SampleScope    string
}

type MarketSummary struct {
	StockCount      int      `json:"stockCount"`
	SampleCount     int      `json:"sampleCount"`
	PositivePECount int      `json:"positivePeCount"`
	ChartStockCount int      `json:"chartStockCount"`
	TotalAmountYi   *float64 `json:"totalAmountYi"`
	UpCount         int      `json:"upCount"`
	DownCount       int      `json:"downCount"`
	FlatCount       int      `json:"flatCount"`
	UpRatio         *float64 `json:"upRatio"`
}

type TopStock struct {
	Code     string   `json:"code"`
	Symbol   string   `json:"symbol"`
	Name     string   `json:"name"`
	PE       *float64 `json:"pe"`
	AmountYi *float64 `json:"amountYi"`
	PctChg   *float64 `json:"pctChg"`
}

type POCBin struct {
	Key           string     `json:"key"`
	Left          float64    `json:"left"`
	Right         float64    `json:"right"`
	StockCount    int        `json:"stockCount"`
	TotalAmount   float64    `json:"-"`
	TotalAmountYi *float64   `json:"totalAmountYi"`
	AvgPctChg     *float64   `json:"avgPctChg"`
	TopStocks     []TopStock `json:"topStocks"`
}

type Payload struct {
	Source             string        `json:"source"`
	SourceNote         string        `json:"sourceNote"`
	UpdatedAt          string        `json:"updatedAt"`
	RefreshHintSeconds int           `json:"refreshHintSeconds"`
	SampleScope        string        `json:"sampleScope"`
	CacheStatus        string        `json:"cacheStatus"`
	LastError          string        `json:"lastError,omitempty"`
	Market             MarketSummary `json:"market"`
	Stocks             []Stock       `json:"stocks"`
	Sectors            []Sector      `json:"sectors"`
	InflowSectors      []Sector      `json:"inflowSectors"`
	POC                *POCBin       `json:"poc"`
	POCDistribution    []POCBin      `json:"pocDistribution"`
}
