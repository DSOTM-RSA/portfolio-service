package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"google.golang.org/api/iterator"
)

const fmpApiKey = "4u2Cubq7CLRKNfVh8JUUPp3exZRO5apO"

// Budget holds the current dynamic budget amount.
type Budget struct {
	Amount float64 `firestore:"amount"`
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
}

// InvestmentLog matches the structure of a document in the 'investment_logs' collection.
type InvestmentLog struct {
	ID               string    `firestore:"-"`
	Ticker           string    `firestore:"ticker"`
	Name             string    `firestore:"name"`
	InvestmentAmount float64   `firestore:"investmentAmount"`
	PricePerShare    float64   `firestore:"pricePerShare"`
	QuantityBought   float64   `firestore:"quantityBought"`
	Strategy         string    `firestore:"strategy"`
	Timestamp        time.Time `firestore:"timestamp"`
}

var (
	key   = []byte("super-secret-yek-12345678901234")
	store = sessions.NewCookieStore(key)
	users = map[string]string{
		"admin": "flexyLion500",
	}
)

var (
	firestoreClient *firestore.Client
	projectID       = "portfolio-468119"
)

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
	}

	// Get the port from the environment variable for Cloud Run
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default to 8080 if PORT is not set
	}

	// Start the server, listening on 0.0.0.0:{port}
	router.Run(":" + port)

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

	// --- NEW: Fetch or Create the Budget ---
	budgetDocRef := firestoreClient.Collection("settings").Doc("budget")
	doc, err := budgetDocRef.Get(ctx)
	var currentBudget Budget
	if err != nil {
		// If document doesn't exist, create it with the default value
		log.Printf("Budget document not found, creating with default value...")
		currentBudget = Budget{Amount: 100.0}
		_, setErr := budgetDocRef.Set(ctx, currentBudget)
		if setErr != nil {
			log.Printf("Failed to create budget document: %v", setErr)
		}
	} else {
		doc.DataTo(&currentBudget)
	}

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": nil,
		"currentBudget": currentBudget.Amount, // Pass budget to the template
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

// Add this new handler function
func showLogsPage(c *gin.Context) {
	ctx := context.Background()
	var logs []InvestmentLog

	// Fetch all documents from the 'investment_logs' collection, ordered by timestamp
	iter := firestoreClient.Collection("investment_logs").OrderBy("timestamp", firestore.Desc).Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate logs: %v", err)
			c.String(http.StatusInternalServerError, "Failed to fetch logs")
			return
		}

		var logEntry InvestmentLog
		if err := doc.DataTo(&logEntry); err != nil {
			log.Printf("Failed to convert log document: %v", err)
			continue
		}
		logEntry.ID = doc.Ref.ID
		logs = append(logs, logEntry)
	}

	// --- Calculate Metrics ---
	totalAmount := 0.0
	investmentCounts := make(map[string]int)
	investmentValues := make(map[string]float64)

	for _, logEntry := range logs {
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

	// Render the logs page with the data and metrics
	c.HTML(http.StatusOK, "logs.tmpl.html", gin.H{
		"Logs":                 logs,
		"TotalInvestments":     len(logs),
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
	budgetDocRef := firestoreClient.Collection("settings").Doc("budget")

	// Update the 'amount' field in the 'budget' document
	_, err := budgetDocRef.Update(ctx, []firestore.Update{
		{Path: "amount", Value: amount},
	})
	if err != nil {
		log.Printf("Failed to update budget: %v", err)
	}

	c.Redirect(http.StatusFound, "/")
}

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

	// Fetch portfolio to display alongside search results
	var stocks []Stock
	iter := firestoreClient.Collection("portfolio").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate for search page: %v", err)
			c.Redirect(http.StatusFound, "/")
			return
		}
		var stock Stock
		doc.DataTo(&stock)
		stocks = append(stocks, stock)
	}

	// --- FIX: Fetch the budget so it doesn't disappear ---
	budgetDocRef := firestoreClient.Collection("settings").Doc("budget")
	doc, err := budgetDocRef.Get(ctx)
	if err != nil {
		// If budget isn't found, just redirect, the main page will create it.
		c.Redirect(http.StatusFound, "/")
		return
	}
	var currentBudget Budget
	doc.DataTo(&currentBudget)

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": results,
		"currentBudget": currentBudget.Amount, // Pass the budget to the template
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

func addStock(c *gin.Context) {
	var newStock Stock

	// Bind the form data to the newStock struct
	if err := c.Bind(&newStock); err != nil {
		c.HTML(http.StatusBadRequest, "bad request: %v", err)
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

		currentPrice, ma200, err := fetchAndAnalyzeStock(stock.Ticker)
		if err != nil {
			fmt.Printf("Could not analyze %s: %v\n", stock.Ticker, err)
			continue
		}

		// Update the struct with new data
		stock.CurrentPrice = currentPrice
		stock.MA200 = ma200
		stock.IsBelowMA = currentPrice < ma200

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

	c.Redirect(http.StatusFound, "/")
}

func handleAllocation(c *gin.Context) {
	ctx := context.Background()

	// 1. Fetch the DYNAMIC BUDGET
	budgetDocRef := firestoreClient.Collection("settings").Doc("budget")
	doc, err := budgetDocRef.Get(ctx)
	if err != nil {
		log.Printf("Could not fetch budget for allocation: %v", err)
		c.Redirect(http.StatusFound, "/")
		return
	}
	var currentBudget Budget
	doc.DataTo(&currentBudget)
	budget := currentBudget.Amount

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

		// Reset budget after allocation
		_, err := budgetDocRef.Update(ctx, []firestore.Update{
			{Path: "amount", Value: 100.0},
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

	c.Redirect(http.StatusFound, "/")
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
