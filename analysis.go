package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type HistoricalPrice struct {
	Date  string  `json:"date"`
	Close float64 `json:"close"`
}

// StockSearchResult matches the FMP search API response structure.
type StockSearchResult struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Exchange string `json:"exchangeShortName"`
}

// searchStocks calls the FMP API to find stocks by name or ticker.
func searchStocks(query string) ([]StockSearchResult, error) {
	// We'll limit results to 10 for efficiency.
	// 2. MODIFY THIS LINE to use url.QueryEscape
	url := fmt.Sprintf("https://financialmodelingprep.com/api/v3/search?query=%s&limit=10&apikey=%s", url.QueryEscape(query), fmpApiKey)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status from API: %s", resp.Status)
	}

	var results []StockSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	return results, nil
}

func calculateSMA(prices []HistoricalPrice, period int) (float64, error) {
	if len(prices) < period {
		return 0, fmt.Errorf("not enough data tocalculate %d-day SMA", period)
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += prices[i].Close
	}

	return sum / float64(period), nil

}

// The function signature now includes a new return value: []HistoricalPrice
func fetchAndAnalyzeStock(ticker string) (currentPrice float64, ma200 float64, historicalData []HistoricalPrice, err error) {
	url := fmt.Sprintf("https://financialmodelingprep.com/api/v3/historical-price-full/%s?timeseries=250&apikey=%s", ticker, fmpApiKey)

	resp, err := http.Get(url)
	if err != nil {
		// Return nil for the new historicalData slice on error
		return 0, 0, nil, fmt.Errorf("failed to get data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, nil, fmt.Errorf("bad status from API: %s", resp.Status)
	}

	var result struct {
		Symbol     string            `json:"symbol"`
		Historical []HistoricalPrice `json:"historical"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	// Assign the fetched data to the new return variable
	historicalData = result.Historical

	if len(historicalData) == 0 {
		return 0, 0, historicalData, fmt.Errorf("no historical prices found for %s", ticker)
	}

	currentPrice = historicalData[0].Close

	ma200, err = calculateSMA(historicalData, 200)
	if err != nil {
		// Return the historical data even if SMA calculation fails, but also return the error
		return currentPrice, 0, historicalData, fmt.Errorf("failed to calculate SMA: %w", err)
	}

	// Return all three values on success
	return currentPrice, ma200, historicalData, nil
}
