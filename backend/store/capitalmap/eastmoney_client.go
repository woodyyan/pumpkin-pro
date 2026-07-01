package capitalmap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	eastmoneyToken     = "bd1d9ddb04089700cf9c27f6f7426281"
	eastmoneyStockURL  = "https://82.push2.eastmoney.com/api/qt/clist/get"
	eastmoneySectorURL = "https://push2.eastmoney.com/api/qt/clist/get"
	eastmoneyTimeout   = 9 * time.Second
	eastmoneyPageSize  = 100
	eastmoneyPageCount = 16
	// batchSize controls how many pages are fetched serially before a pause.
	// Fetching all 16 pages concurrently triggers IP-based rate limiting on the
	// eastmoney servers when requests originate from overseas IPs.
	eastmoneyBatchSize    = 4
	eastmoneyBatchPause   = 800 * time.Millisecond
	eastmoneyUserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36"
	eastmoneyReferer      = "https://quote.eastmoney.com/"
	eastmoneyPageInterval = 200 * time.Millisecond
)

var stockFields = []string{
	"f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10", "f12", "f13", "f14", "f15", "f16", "f17", "f18", "f20", "f21", "f23", "f24", "f25", "f62", "f115",
}

var sectorFields = []string{"f3", "f6", "f12", "f14", "f62", "f128", "f140", "f141"}

type EastmoneyClient struct {
	httpClient *http.Client
	now        func() time.Time
	// sleep is injectable for tests.
	sleep func(time.Duration)
}

func NewEastmoneyClient(httpClient *http.Client) *EastmoneyClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: eastmoneyTimeout + time.Second}
	}
	return &EastmoneyClient{httpClient: httpClient, now: time.Now, sleep: time.Sleep}
}

// FetchAshareSnapshot fetches all pages serially in small batches.
// Pages are fetched one by one with a short pause between each request
// and a longer pause between batches, to avoid triggering rate-limiting
// or IP bans on overseas servers. A single page failure is logged and
// skipped; the remaining pages are still collected.
func (c *EastmoneyClient) FetchAshareSnapshot(ctx context.Context) (SnapshotResult, error) {
	allRows := make([]map[string]any, 0, eastmoneyPageCount*eastmoneyPageSize)
	totalAvailable := 0
	failedPages := 0

	for page := 1; page <= eastmoneyPageCount; page++ {
		if ctx.Err() != nil {
			return SnapshotResult{}, ctx.Err()
		}

		var payload eastmoneyListResponse
		if err := c.fetchJSON(ctx, c.stockURL(page), &payload); err != nil {
			log.Printf("eastmoney stock page %d fetch failed (skipped): %v", page, err)
			failedPages++
		} else {
			if totalAvailable == 0 {
				totalAvailable = payload.Data.Total
			}
			allRows = append(allRows, payload.Data.Diff...)
		}

		// Pause between individual pages.
		if page < eastmoneyPageCount {
			c.sleep(eastmoneyPageInterval)
		}

		// Extra pause at the end of each batch (except after the last page).
		if page%eastmoneyBatchSize == 0 && page < eastmoneyPageCount {
			c.sleep(eastmoneyBatchPause)
		}
	}

	if len(allRows) == 0 {
		return SnapshotResult{}, fmt.Errorf("eastmoney: all %d pages failed", eastmoneyPageCount)
	}

	stocks := make([]Stock, 0, len(allRows))
	for _, item := range allRows {
		stock := stockFromEastmoneyRow(item)
		if stock.Code == "" || stock.Name == "" || stock.Amount <= 0 {
			continue
		}
		stocks = append(stocks, stock)
	}

	scope := fmt.Sprintf("成交额前 %d 只股票", len(stocks))
	if failedPages > 0 {
		scope += fmt.Sprintf("（%d 页获取失败）", failedPages)
	}
	return SnapshotResult{
		Stocks:         stocks,
		TotalAvailable: totalAvailable,
		SampleScope:    scope,
	}, nil
}

func (c *EastmoneyClient) FetchIndustrySectors(ctx context.Context) ([]Sector, error) {
	var payload eastmoneyListResponse
	if err := c.fetchJSON(ctx, c.sectorURL(), &payload); err != nil {
		return nil, err
	}
	sectors := make([]Sector, 0, len(payload.Data.Diff))
	for _, item := range payload.Data.Diff {
		sector := sectorFromEastmoneyRow(item)
		if sector.Code == "" || sector.Name == "" || sector.Amount <= 0 {
			continue
		}
		sectors = append(sectors, sector)
	}
	return sectors, nil
}

func (c *EastmoneyClient) stockURL(page int) string {
	values := url.Values{}
	values.Set("pn", fmt.Sprintf("%d", page))
	values.Set("pz", fmt.Sprintf("%d", eastmoneyPageSize))
	values.Set("po", "1")
	values.Set("np", "1")
	values.Set("ut", eastmoneyToken)
	values.Set("fltt", "2")
	values.Set("invt", "2")
	values.Set("fid", "f6")
	values.Set("fs", "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048")
	values.Set("fields", strings.Join(stockFields, ","))
	values.Set("_", fmt.Sprintf("%d", c.now().UnixMilli()))
	return eastmoneyStockURL + "?" + values.Encode()
}

func (c *EastmoneyClient) sectorURL() string {
	values := url.Values{}
	values.Set("pn", "1")
	values.Set("pz", "80")
	values.Set("po", "1")
	values.Set("np", "1")
	values.Set("ut", eastmoneyToken)
	values.Set("fltt", "2")
	values.Set("invt", "2")
	values.Set("fid", "f62")
	values.Set("fs", "m:90+t:2+f:!50")
	values.Set("fields", strings.Join(sectorFields, ","))
	values.Set("_", fmt.Sprintf("%d", c.now().UnixMilli()))
	return eastmoneySectorURL + "?" + values.Encode()
}

func (c *EastmoneyClient) fetchJSON(ctx context.Context, targetURL string, output any) error {
	requestCtx, cancel := context.WithTimeout(ctx, eastmoneyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", eastmoneyUserAgent)
	req.Header.Set("Referer", eastmoneyReferer)
	req.Header.Set("Accept", "application/json,text/plain,*/*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("eastmoney request failed: %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(output); err != nil {
		return err
	}
	return nil
}

type eastmoneyListResponse struct {
	Data struct {
		Total int              `json:"total"`
		Diff  []map[string]any `json:"diff"`
	} `json:"data"`
}

func stockFromEastmoneyRow(item map[string]any) Stock {
	code := fieldString(item, "f12")
	dynamicPE := fieldFloat(item, "f9")
	peTTM := fieldFloat(item, "f115")
	selectedPE := dynamicPE
	peSource := "动态PE"
	if peTTM != nil && *peTTM > 0 {
		selectedPE = peTTM
		peSource = "PE TTM"
	}
	amount := deref(fieldFloat(item, "f6"))
	market := marketPrefix(code, fieldFloat(item, "f13"))
	mainNetInflow := fieldFloat(item, "f62")
	marketCap := fieldFloat(item, "f20")
	floatMarketCap := fieldFloat(item, "f21")

	return Stock{
		Code:             code,
		Symbol:           market + code,
		Name:             fieldString(item, "f14"),
		Market:           market,
		Price:            fieldFloat(item, "f2"),
		PctChg:           fieldFloat(item, "f3"),
		Amount:           amount,
		AmountYi:         roundPtr(amount/100000000, 2),
		VolumeHands:      fieldFloat(item, "f5"),
		TurnoverRate:     fieldFloat(item, "f8"),
		PE:               selectedPE,
		PETTM:            peTTM,
		DynamicPE:        dynamicPE,
		PESource:         peSource,
		PB:               fieldFloat(item, "f23"),
		TotalMarketCap:   marketCap,
		FloatMarketCap:   floatMarketCap,
		TotalMarketCapYi: roundPtr(deref(marketCap)/100000000, 2),
		FloatMarketCapYi: roundPtr(deref(floatMarketCap)/100000000, 2),
		MainNetInflow:    mainNetInflow,
		MainNetInflowYi:  roundPtr(deref(mainNetInflow)/100000000, 2),
		Change60D:        fieldFloat(item, "f24"),
		ChangeYTD:        fieldFloat(item, "f25"),
	}
}

func sectorFromEastmoneyRow(item map[string]any) Sector {
	amount := deref(fieldFloat(item, "f6"))
	mainNetInflow := deref(fieldFloat(item, "f62"))
	var intensity *float64
	if amount > 0 {
		intensity = roundPtr((mainNetInflow/amount)*100, 2)
	}
	return Sector{
		Code:               fieldString(item, "f12"),
		Name:               fieldString(item, "f14"),
		PctChg:             fieldFloat(item, "f3"),
		Amount:             amount,
		AmountYi:           roundPtr(amount/100000000, 2),
		MainNetInflow:      mainNetInflow,
		MainNetInflowYi:    roundPtr(mainNetInflow/100000000, 2),
		NetInflowIntensity: intensity,
		LeaderName:         fieldString(item, "f128"),
		LeaderCode:         fieldString(item, "f140"),
	}
}

func marketPrefix(code string, market *float64) string {
	if strings.HasPrefix(code, "6") || deref(market) == 1 {
		return "SH"
	}
	if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "9") {
		return "BJ"
	}
	return "SZ"
}

func fieldString(item map[string]any, key string) string {
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		if typed == "-" {
			return ""
		}
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func fieldFloat(item map[string]any, key string) *float64 {
	value, ok := item[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case float64:
		return &typed
	case json.Number:
		if parsed, err := typed.Float64(); err == nil {
			return &parsed
		}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" || text == "-" {
			return nil
		}
		parsed, err := json.Number(text).Float64()
		if err == nil {
			return &parsed
		}
	}
	return nil
}
