package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"google.golang.org/api/iterator"
)

// The new Settings struct
type Settings struct {
	Amount          float64 `firestore:"amount"`
	NextBatchNumber int     `firestore:"nextBatchNumber"`
}

// Stock represents data about a stock.
type Stock struct {
	Ticker         string  `json:"ticker" form:"ticker"`
	Name           string  `json:"name" form:"name"`
	Quantity       float64 `json:"quantity" form:"quantity"`
	Price          float64 `json:"price" form:"price"`
	CurrentPrice   float64 `json:"current_price"`
	MA200          float64 `json:"ma_200"`
	IsBelowMA      bool    `json:"is_below_ma"`
	Recommendation string  `json:"recommendation"`
	EMATrend       float64 `json:"ema_trend"`
}

// InvestmentLog matches the structure of a document in the 'investment_logs' collection.
type InvestmentLog struct {
	ID               string    `firestore:"-"`
	Batch            int       `firestore:"batch"`
	Ticker           string    `firestore:"ticker"`
	Name             string    `firestore:"name"`
	InvestmentAmount float64   `firestore:"investmentAmount"`
	PricePerShare    float64   `firestore:"pricePerShare"`
	QuantityBought   float64   `firestore:"quantityBought"`
	Strategy         string    `firestore:"strategy"`
	Timestamp        time.Time `firestore:"timestamp"`
}

// PortfolioHistoryPoint represents the value of both strategies at a single point in time.
type PortfolioHistoryPoint struct {
	Date       int64   `json:"date"`
	MAValue    float64 `json:"maValue"`
	NaiveValue float64 `json:"naiveValue"`
}

var (
	key   = []byte("super-secret-yek-12345678901234")
	store = sessions.NewCookieStore(key)
	users = make(map[string]string)
)

var (
	firestoreClient *firestore.Client
	projectID       string
	fmpApiKey       string
)

func init() {
	// Attempt to load the .env file.
	// This will not return an error if the file doesn't exist,
	// which is perfect for production environments.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables from OS")
	}

	projectID = os.Getenv("PROJECT_ID")
	if projectID == "" {
		log.Fatal("PROJECT_ID must be set")
	}

	fmpApiKey = os.Getenv("FMP_API_KEY")
	if fmpApiKey == "" {
		log.Fatal("FMP_API_KEY must be set")
	}

	// 2. READ the admin password from the environment.
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		log.Fatal("ADMIN_PASSWORD must be set")
	}

	// 3. POPULATE the users map with the loaded password.
	users["admin"] = adminPassword
}

func createFirestoreClient(ctx context.Context) *firestore.Client {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	return client
}

func main() {
	ctx := context.Background()
	firestoreClient = createFirestoreClient(ctx)
	defer firestoreClient.Close()
	router := gin.Default()

	// Tell Gin to load HTML templates form the "tempaltes" drectory
	router.LoadHTMLGlob("templates/*")

	// Routes for login
	router.GET("/login", showLoginPage)
	router.POST("/login", handleLogin)
	router.POST("/logout", handleLogout)

	// Group protected routes that require login
	protected := router.Group("/")
	protected.Use(authMiddleware())
	{
		protected.GET("/logs", showLogsPage)
		protected.GET("/", showPortfolioPage)
		protected.GET("/search", handleSearch)
		protected.POST("/add-stock", addStock)
		protected.POST("/delete", handleDelete)
		protected.POST("/update", handleUpdate)
		protected.POST("/analyze", handleAnalysis)
		protected.POST("/allocate", handleAllocation)
		protected.POST("/update-budget", handleUpdateBudget)
		protected.POST("/logs/delete", handleDeleteLog)
		protected.GET("/chart", showChartPage)
		protected.GET("/api/portfolio-history", handlePortfolioHistory)
	}

	// Get the port from the environment variable for Cloud Run
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default to 8080 if PORT is not set
	}

	// Start the server, listening on 0.0.0.0:{port}
	router.Run(":" + port)

}

func showChartPage(c *gin.Context) {
	// Create a slice of dummy data points for testing
	dummyHistory := []PortfolioHistoryPoint{
		{Date: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), MAValue: 100, NaiveValue: 100},
		{Date: time.Date(2025, 7, 8, 0, 0, 0, 0, time.UTC).UnixMilli(), MAValue: 105, NaiveValue: 102},
		{Date: time.Date(2025, 7, 15, 0, 0, 0, 0, time.UTC).UnixMilli(), MAValue: 112, NaiveValue: 108},
		{Date: time.Date(2025, 7, 22, 0, 0, 0, 0, time.UTC).UnixMilli(), MAValue: 110, NaiveValue: 115},
		{Date: time.Date(2025, 7, 29, 0, 0, 0, 0, time.UTC).UnixMilli(), MAValue: 120, NaiveValue: 118},
	}

	dummyDataJSON, err := json.Marshal(dummyHistory)
	if err != nil {
		log.Printf("Failed to marshal dummy data: %v", err)
		c.String(http.StatusInternalServerError, "Failed to generate chart data")
		return
	}

	// Pass the dummy data directly to the template
	c.HTML(http.StatusOK, "chart.tmpl.html", gin.H{
		"dummyData": template.JS(dummyDataJSON),
	})
}

// showPortfolioPage renders the portfolio page with the current stock data.
func showPortfolioPage(c *gin.Context) {
	ctx := context.Background()
	var stocks []Stock
	// ... (your existing code to fetch stocks is the same) ...
	iter := firestoreClient.Collection("portfolio").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil { /* ... error handling ... */
		}
		var stock Stock
		doc.DataTo(&stock)
		stocks = append(stocks, stock)
	}

	settingsDocRef := firestoreClient.Collection("settings").Doc("app") // Use a clear doc ID
	doc, err := settingsDocRef.Get(ctx)
	var currentSettings Settings
	if err != nil {
		log.Printf("Settings document not found, creating with default values...")
		currentSettings = Settings{Amount: 100.0, NextBatchNumber: 1} // Default settings
		_, setErr := settingsDocRef.Set(ctx, currentSettings)
		if setErr != nil {
			log.Printf("Failed to create settings document: %v", setErr)
		}
	} else {
		doc.DataTo(&currentSettings)
	}

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": nil,
		"currentBudget": currentSettings.Amount, // Pass budget amount to template
	})
}

func handleDeleteLog(c *gin.Context) {
	logID := c.PostForm("logID")
	if logID == "" {
		c.String(http.StatusBadRequest, "Log ID is required")
		return
	}

	ctx := context.Background()
	// Delete the document with the matching ID from the 'investment_logs' collection
	_, err := firestoreClient.Collection("investment_logs").Doc(logID).Delete(ctx)
	if err != nil {
		log.Printf("Failed to delete log entry %s: %v", logID, err)
		c.String(http.StatusInternalServerError, "Failed to delete log entry")
		return
	}

	// Redirect back to the logs page
	c.Redirect(http.StatusFound, "/logs")
}

func handleDeleteLogBatch(c *gin.Context) {
	batchStr := c.PostForm("batch")
	batch, _ := strconv.Atoi(batchStr)
	if batch == 0 {
		c.String(http.StatusBadRequest, "Invalid batch number")
		return
	}

	ctx := context.Background()
	// Find all documents in the naive logs collection with the matching batch number
	query := firestoreClient.Collection("naive_strategy_logs").Where("batch", "==", batch)
	iter := query.Documents(ctx)

	// Use a batched write to delete all found documents efficiently
	batchWrite := firestoreClient.Batch()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate for batch delete: %v", err)
			break
		}
		batchWrite.Delete(doc.Ref)
	}

	// Commit the batched delete operation
	_, err := batchWrite.Commit(ctx)
	if err != nil {
		log.Printf("Failed to commit batch delete for batch %d: %v", batch, err)
	}

	c.Redirect(http.StatusFound, "/logs")
}

func showLogsPage(c *gin.Context) {
	ctx := context.Background()

	logBatches := make(map[int][]InvestmentLog)
	var allLogs []InvestmentLog

	collections := []string{"investment_logs", "naive_strategy_logs", "ema_logs"}

	for _, coll := range collections {
		iter := firestoreClient.Collection(coll).OrderBy("batch", firestore.Desc).OrderBy("timestamp", firestore.Desc).Documents(ctx)
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Printf("Failed to iterate logs from %s: %v", coll, err)
				continue // Continue to the next document or collection
			}

			var logEntry InvestmentLog
			if err := doc.DataTo(&logEntry); err != nil {
				log.Printf("Failed to convert log document from %s: %v", coll, err)
				continue
			}
			logEntry.ID = doc.Ref.ID

			logBatches[logEntry.Batch] = append(logBatches[logEntry.Batch], logEntry)
			allLogs = append(allLogs, logEntry)
		}
	}

	// --- Calculate Metrics from all logs ---
	totalAmount := 0.0
	investmentCounts := make(map[string]int)
	investmentValues := make(map[string]float64)

	for _, logEntry := range allLogs {
		totalAmount += logEntry.InvestmentAmount
		investmentCounts[logEntry.Ticker]++
		investmentValues[logEntry.Ticker] += logEntry.InvestmentAmount
	}

	mostFrequentStock, mostFrequentCount := "", 0
	for ticker, count := range investmentCounts {
		if count > mostFrequentCount {
			mostFrequentCount = count
			mostFrequentStock = ticker
		}
	}

	highestInvestedStock, highestInvestedValue := "", 0.0
	for ticker, value := range investmentValues {
		if value > highestInvestedValue {
			highestInvestedValue = value
			highestInvestedStock = ticker
		}
	}

	// Render the logs page with the grouped data and the overall metrics
	c.HTML(http.StatusOK, "logs.tmpl.html", gin.H{
		"LogBatches":           logBatches,
		"TotalInvestments":     len(allLogs),
		"TotalAmount":          totalAmount,
		"MostFrequentStock":    mostFrequentStock,
		"MostFrequentCount":    mostFrequentCount,
		"HighestInvestedStock": highestInvestedStock,
		"HighestInvestedValue": highestInvestedValue,
	})
}

func handleUpdateBudget(c *gin.Context) {
	amountStr := c.PostForm("amount")
	amount, _ := strconv.ParseFloat(strings.Replace(amountStr, ",", ".", -1), 64)

	ctx := context.Background()
	// FIX: Use the 'app' document ID for consistency
	settingsDocRef := firestoreClient.Collection("settings").Doc("app")

	// Update the 'amount' field in the document
	_, err := settingsDocRef.Update(ctx, []firestore.Update{
		{Path: "amount", Value: amount},
	})
	if err != nil {
		log.Printf("Failed to update budget: %v", err)
	}

	c.Redirect(http.StatusFound, "/")
}

// main.go

func handleSearch(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	results, err := searchStocks(query)
	if err != nil {
		fmt.Printf("Error searching stocks: %v\n", err)
		c.Redirect(http.StatusFound, "/")
		return
	}

	ctx := context.Background()

	// ... (fetching portfolio stocks is the same) ...
	var stocks []Stock
	iter := firestoreClient.Collection("portfolio").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil { /* ... error handling ... */
		}
		var stock Stock
		doc.DataTo(&stock)
		stocks = append(stocks, stock)
	}

	// --- FIX: Use 'Settings' struct and 'app' document ID ---
	settingsDocRef := firestoreClient.Collection("settings").Doc("app")
	doc, err := settingsDocRef.Get(ctx)
	if err != nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	var currentSettings Settings // Use the correct Settings struct
	doc.DataTo(&currentSettings)

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": results,
		"currentBudget": currentSettings.Amount, // Pass the amount to the template
	})
}

func handleDelete(c *gin.Context) {
	ticker := c.PostForm("ticker")
	if ticker == "" {
		c.String(http.StatusBadRequest, "Ticker is required")
		return
	}

	ctx := context.Background()
	// Delete the document with the matching ticker ID
	_, err := firestoreClient.Collection("portfolio").Doc(ticker).Delete(ctx)
	if err != nil {
		log.Printf("Failed to delete stock %s: %v", ticker, err)
		c.String(http.StatusInternalServerError, "Failed to delete stock")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// main.go

func handleUpdate(c *gin.Context) {
	// The ticker now comes from the button's value, not a query parameter
	ticker := c.PostForm("ticker")
	quantityStr := c.PostForm("quantity")
	priceStr := c.PostForm("price")

	// Convert string values, handling potential commas from different locales
	quantity, _ := strconv.ParseFloat(strings.Replace(quantityStr, ",", ".", -1), 64)
	price, _ := strconv.ParseFloat(strings.Replace(priceStr, ",", ".", -1), 64)

	if ticker == "" {
		c.String(http.StatusBadRequest, "Ticker is required")
		return
	}

	ctx := context.Background()
	// Update specific fields in the document
	_, err := firestoreClient.Collection("portfolio").Doc(ticker).Update(ctx, []firestore.Update{
		{Path: "Quantity", Value: quantity},
		{Path: "Price", Value: price},
	})

	if err != nil {
		log.Printf("Failed to update stock %s: %v", ticker, err)
		c.String(http.StatusInternalServerError, "Failed to update stock")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// main.go

func addStock(c *gin.Context) {
	var newStock Stock

	// Use ShouldBind for consistency with other handlers
	if err := c.ShouldBind(&newStock); err != nil {
		c.String(http.StatusBadRequest, "bad request: %v", err)
		return
	}

	ctx := context.Background()
	// Use the Ticker as the document ID in the "portfolio" collection
	_, err := firestoreClient.Collection("portfolio").Doc(newStock.Ticker).Set(ctx, newStock)
	if err != nil {
		log.Printf("Failed to add stock: %v", err)
		c.String(http.StatusInternalServerError, "Failed to add stock")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

func handleAnalysis(c *gin.Context) {
	ctx := context.Background()
	iter := firestoreClient.Collection("portfolio").Documents(ctx)

	// This can be slow! In a real app, this would be a background job.
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate for analysis: %v", err)
			c.Redirect(http.StatusFound, "/")
			return
		}

		var stock Stock
		doc.DataTo(&stock)

		currentPrice, ma200, emaTrend, _, err := fetchAndAnalyzeStock(stock.Ticker)
		if err != nil {
			fmt.Printf("Could not analyze %s: %v\n", stock.Ticker, err)
			continue
		}

		// Update the struct with new data
		stock.CurrentPrice = currentPrice
		stock.MA200 = ma200
		stock.IsBelowMA = currentPrice < ma200
		stock.EMATrend = emaTrend

		// Save the updated stock data back to Firestore
		_, err = firestoreClient.Collection("portfolio").Doc(stock.Ticker).Set(ctx, stock)
		if err != nil {
			log.Printf("Failed to update stock %s: %v", stock.Ticker, err)
		}
	}

	// Log the interaction to a separate "logs" collection
	_, _, err := firestoreClient.Collection("logs").Add(ctx, map[string]interface{}{
		"action":    "Portfolio Analyzed",
		"timestamp": time.Now(),
	})
	if err != nil {
		log.Printf("Failed to add log entry: %v", err)
	}

	c.Redirect(http.StatusFound, "/?status=analyzed")
}

func handleAllocation(c *gin.Context) {
	ctx := context.Background()

	// 1. Fetch the DYNAMIC SETTINGS (Budget and Batch Number)
	settingsDocRef := firestoreClient.Collection("settings").Doc("app")
	doc, err := settingsDocRef.Get(ctx)
	if err != nil {
		log.Printf("Could not fetch budget for allocation: %v", err)
		c.Redirect(http.StatusFound, "/")
		return
	}
	var currentSettings Settings
	doc.DataTo(&currentSettings)

	budget := currentSettings.Amount
	batchNumber := currentSettings.NextBatchNumber // Get the current batch number

	// 2. Fetch all stocks from Firestore
	var portfolioStocks []*Stock
	iter := firestoreClient.Collection("portfolio").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate for allocation: %v", err)
			c.Redirect(http.StatusFound, "/")
			return
		}
		var stock Stock
		doc.DataTo(&stock)
		portfolioStocks = append(portfolioStocks, &stock)
	}

	// 3. Primary Strategy
	var eligibleStocks []*Stock
	var totalScore float64
	for _, stock := range portfolioStocks {
		if stock.IsBelowMA && stock.MA200 > 0 && stock.CurrentPrice > 0 {
			score := stock.MA200 - stock.CurrentPrice
			totalScore += score
			eligibleStocks = append(eligibleStocks, stock)
		}
	}

	// 4. Handle Rollover or Allocation
	if len(eligibleStocks) == 0 {
		log.Println("No eligible stocks for investment. Budget will roll over.")
	} else {
		if totalScore > 0 {
			recommendedTickers := make(map[string]bool)

			for _, stock := range eligibleStocks {
				score := stock.MA200 - stock.CurrentPrice
				weight := score / totalScore
				investmentAmount := budget * weight
				quantityToBuy := investmentAmount / stock.CurrentPrice
				recommendationText := fmt.Sprintf("Invest €%.2f", investmentAmount)

				recommendedTickers[stock.Ticker] = true

				totalOldValue := stock.Price * stock.Quantity
				newTotalQuantity := stock.Quantity + quantityToBuy
				newAveragePrice := (totalOldValue + investmentAmount) / newTotalQuantity

				newTotalQuantity = math.Round(newTotalQuantity*100) / 100

				// --- FIX: Remove the int() conversion for Quantity ---
				_, err := firestoreClient.Collection("portfolio").Doc(stock.Ticker).Update(ctx, []firestore.Update{
					{Path: "Quantity", Value: newTotalQuantity}, // Correctly pass the float64
					{Path: "Price", Value: newAveragePrice},
					{Path: "Recommendation", Value: recommendationText},
				})
				if err != nil {
					log.Printf("Failed to auto-update portfolio for %s: %v", stock.Ticker, err)
				}

				// Log to 'investment_logs'
				// This part is correct and doesn't need changes.
				_, _, err = firestoreClient.Collection("investment_logs").Add(ctx, map[string]interface{}{
					"batch":            batchNumber, // <-- ADD BATCH NUMBER
					"ticker":           stock.Ticker,
					"name":             stock.Name,
					"investmentAmount": investmentAmount,
					"pricePerShare":    stock.CurrentPrice,
					"quantityBought":   quantityToBuy,
					"strategy":         "200-Day MA Undervalued",
					"timestamp":        time.Now(),
				})
				if err != nil {
					log.Printf("Failed to add investment log for %s: %v", stock.Ticker, err)
				}
			}

			// Clear recommendations for non-eligible stocks
			for _, stock := range portfolioStocks {
				if !recommendedTickers[stock.Ticker] {
					firestoreClient.Collection("portfolio").Doc(stock.Ticker).Update(ctx, []firestore.Update{
						{Path: "Recommendation", Value: ""},
					})
				}
			}
		}

		// After a successful allocation, increment the batch number and reset the budget
		_, err := settingsDocRef.Update(ctx, []firestore.Update{
			{Path: "amount", Value: 100.0},
			{Path: "nextBatchNumber", Value: batchNumber + 1}, // <-- INCREMENT BATCH
		})
		if err != nil {
			log.Printf("Failed to reset budget after allocation: %v", err)
		}
	}

	// 5. Naive Strategy (This part remains the same, just logging)
	var totalPortfolioValue float64
	// Recalculate total value based on original quantities for fair comparison
	for _, stock := range portfolioStocks {
		totalPortfolioValue += stock.CurrentPrice * float64(stock.Quantity)
	}

	if totalPortfolioValue > 0 {
		for _, stock := range portfolioStocks {
			stockValue := stock.CurrentPrice * float64(stock.Quantity)
			weight := stockValue / totalPortfolioValue
			investmentAmount := budget * weight
			quantityToBuy := investmentAmount / stock.CurrentPrice

			// Log this transaction to 'naive_strategy_logs'
			_, _, err := firestoreClient.Collection("naive_strategy_logs").Add(ctx, map[string]interface{}{
				"batch":            batchNumber,
				"ticker":           stock.Ticker,
				"name":             stock.Name,
				"investmentAmount": investmentAmount,
				"pricePerShare":    stock.CurrentPrice,
				"quantityBought":   quantityToBuy,
				"strategy":         "Naive Proportional Allocation",
				"timestamp":        time.Now(),
			})
			if err != nil {
				log.Printf("Failed to add naive strategy log for %s: %v", stock.Ticker, err)
			}
		}
	}

	// 6. EMA-112 Strategy
	if len(portfolioStocks) >= 3 {
		var mostNegativeStock, firstPositiveStock, secondPositiveStock *Stock
		minEma := math.MaxFloat64
		maxEma1, maxEma2 := -math.MaxFloat64, -math.MaxFloat64

		for _, stock := range portfolioStocks {
			if stock.EMATrend < minEma {
				minEma = stock.EMATrend
				mostNegativeStock = stock
			}
			if stock.EMATrend > maxEma1 {
				maxEma2 = maxEma1
				secondPositiveStock = firstPositiveStock
				maxEma1 = stock.EMATrend
				firstPositiveStock = stock
			} else if stock.EMATrend > maxEma2 {
				maxEma2 = stock.EMATrend
				secondPositiveStock = stock
			}
		}

		// Log the hypothetical "sell" of the most negative stock
		if mostNegativeStock != nil && mostNegativeStock.EMATrend < 0 {
			_, _, err := firestoreClient.Collection("ema_logs").Add(ctx, map[string]interface{}{
				"batch":            batchNumber,
				"ticker":           mostNegativeStock.Ticker,
				"name":             mostNegativeStock.Name,
				"investmentAmount": -mostNegativeStock.CurrentPrice, // Negative for sell
				"pricePerShare":    mostNegativeStock.CurrentPrice,
				"quantityBought":   -1, // Negative for sell
				"strategy":         "ema-approach",
				"timestamp":        time.Now(),
			})
			if err != nil {
				log.Printf("Failed to add EMA strategy sell log for %s: %v", mostNegativeStock.Ticker, err)
			}
		}

		// Log the hypothetical "buy" of the two most positive stocks
		if firstPositiveStock != nil && secondPositiveStock != nil && firstPositiveStock.EMATrend > 0 && secondPositiveStock.EMATrend > 0 {
			totalPositiveEma := firstPositiveStock.EMATrend + secondPositiveStock.EMATrend
			stocksToBuy := []*Stock{firstPositiveStock, secondPositiveStock}

			for _, stock := range stocksToBuy {
				weight := stock.EMATrend / totalPositiveEma
				investmentAmount := budget * weight
				quantityToBuy := investmentAmount / stock.CurrentPrice

				_, _, err := firestoreClient.Collection("ema_logs").Add(ctx, map[string]interface{}{
					"batch":            batchNumber,
					"ticker":           stock.Ticker,
					"name":             stock.Name,
					"investmentAmount": investmentAmount,
					"pricePerShare":    stock.CurrentPrice,
					"quantityBought":   quantityToBuy,
					"strategy":         "ema-approach",
					"timestamp":        time.Now(),
				})
				if err != nil {
					log.Printf("Failed to add EMA strategy buy log for %s: %v", stock.Ticker, err)
				}
			}
		}
	}
	c.Redirect(http.StatusFound, "/?status=allocated")
}

// authMiddleware checks if the user is authenticated
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session, _ := store.Get(c.Request, "session-name")

		// Check if user is authenticated
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			c.Redirect(http.StatusFound, "/login")
			c.Abort() // Stop the request chain
			return
		}

		// If authenticated, proceed to the next handler
		c.Next()
	}
}

func showLoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.tmpl.html", nil)
}

// Checks credentials and creates a session
func handleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	// Check if username and password ar valid
	if expectedPassword, ok := users[username]; ok && expectedPassword == password {
		session, _ := store.Get(c.Request, "session-name")
		session.Values["authenticated"] = true
		session.Save(c.Request, c.Writer)
		c.Redirect(http.StatusFound, "/")
	} else {
		// If login fails, render the login page with an error
		c.HTML(http.StatusUnauthorized, "login.tmpl.html", gin.H{
			"error": "Invalid credentials"})
	}
}

func handleLogout(c *gin.Context) {
	session, _ := store.Get(c.Request, "session-name")
	session.Values["authenticated"] = false
	session.Save(c.Request, c.Writer)
	c.Redirect(http.StatusFound, "/login")
}

func getPriceOnDate(prices []HistoricalPrice, date time.Time) float64 {
	targetDateStr := date.Format("2006-01-02")
	lastKnownPrice := 0.0
	for _, p := range prices {
		if p.Date > targetDateStr {
			break // Stop once we've passed the target date
		}
		lastKnownPrice = p.Close
	}
	return lastKnownPrice
}

func handlePortfolioHistory(c *gin.Context) {
	ctx := context.Background()

	// 1. Fetch all logs for both strategies
	var allLogs []InvestmentLog
	collections := []string{"investment_logs", "naive_strategy_logs"}
	for _, coll := range collections {
		iter := firestoreClient.Collection(coll).OrderBy("timestamp", firestore.Asc).Documents(ctx) // Order Ascending now
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				continue
			}
			var logEntry InvestmentLog
			doc.DataTo(&logEntry)
			allLogs = append(allLogs, logEntry)
		}
	}

	if len(allLogs) == 0 {
		c.JSON(http.StatusOK, []PortfolioHistoryPoint{})
		return
	}

	// 2. Get all unique tickers
	tickers := make(map[string]bool)
	for _, logEntry := range allLogs {
		tickers[logEntry.Ticker] = true
	}

	// 3. Fetch all required historical price data
	priceHistory := make(map[string][]HistoricalPrice)
	for ticker := range tickers {
		_, _, _, historicalData, _ := fetchAndAnalyzeStock(ticker)
		// Reverse the historical data so it's oldest to newest
		for i, j := 0, len(historicalData)-1; i < j; i, j = i+1, j-1 {
			historicalData[i], historicalData[j] = historicalData[j], historicalData[i]
		}
		priceHistory[ticker] = historicalData
	}

	// 4. Reconstruct portfolio values over time
	var history []PortfolioHistoryPoint
	holdingsMA := make(map[string]float64)
	holdingsNaive := make(map[string]float64)
	logIndex := 0

	// Iterate from the first investment day to today
	for d := allLogs[0].Timestamp; !d.After(time.Now()); d = d.AddDate(0, 0, 1) {
		// Add any shares "bought" on this day
		for logIndex < len(allLogs) && !allLogs[logIndex].Timestamp.After(d) {
			logEntry := allLogs[logIndex]
			if logEntry.Strategy == "200-Day MA Undervalued" {
				holdingsMA[logEntry.Ticker] += logEntry.QuantityBought
			} else {
				holdingsNaive[logEntry.Ticker] += logEntry.QuantityBought
			}
			logIndex++
		}

		// We only need to calculate the value at the end of each week (Friday)
		if d.Weekday() != time.Friday {
			continue
		}

		// Calculate total portfolio value using the most recent price available
		var maValue, naiveValue float64
		for ticker, qty := range holdingsMA {
			maValue += qty * getPriceOnDate(priceHistory[ticker], d)
		}
		for ticker, qty := range holdingsNaive {
			naiveValue += qty * getPriceOnDate(priceHistory[ticker], d)
		}

		history = append(history, PortfolioHistoryPoint{
			Date:       d.UnixMilli(),
			MAValue:    maValue,
			NaiveValue: naiveValue,
		})
	}

	c.JSON(http.StatusOK, history)
}
