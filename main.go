package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"google.golang.org/api/iterator"
)

const fmpApiKey = "4u2Cubq7CLRKNfVh8JUUPp3exZRO5apO"

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

// albums slice to seed record album data.
// var portfolio = []Stock{
// 	{Ticker: "GOOGL", Name: "Alphabet Inc.", Quantity: 10.56, Price: 135.50},
// 	{Ticker: "MSFT", Name: "Microsoft Corp.", Quantity: 20.34, Price: 330.80},
// 	{Ticker: "AAPL", Name: "Apple Inc.", Quantity: 15.98, Price: 170.25},
// }

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
		protected.GET("/", showPortfolioPage)
		protected.GET("/search", handleSearch)
		protected.POST("/add-stock", addStock)
		protected.POST("/analyze", handleAnalysis)
		protected.POST("/reweight", handleReweight)
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
	var stocks []Stock // create an empty slice to hold stocks from Firestore

	// Get an iterator for all documents in the "portfolio" collection
	iter := firestoreClient.Collection("portfolio").Documents(ctx)

	// Iterate through the documents
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate: %v", err)
			c.String(http.StatusInternalServerError, "Failed to fetch portfolio")
			return
		}

		// Convert the document into a Stock struct
		var stock Stock
		if err := doc.DataTo(&stock); err != nil {
			log.Printf("Failed to convert document to stock: %v", err)
			continue // Skip to the next documents
		}
		stocks = append(stocks, stock)
	}

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": nil, // Ensure search results are empty on initial load
	})
}

// Add this new handler function to main.go
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

	// Fetch the current portfolio to display alongside search results
	ctx := context.Background()
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

	c.HTML(http.StatusOK, "index.tmpl.html", gin.H{
		"stocks":        stocks,
		"searchResults": results,
	})
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

func handleReweight(c *gin.Context) {
	const budget = 100.0
	var totalScore float64

	ctx := context.Background()

	// 1. Fetch all stocks from Firestore first
	var stocksToUpdate []*Stock
	iter := firestoreClient.Collection("portfolio").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("Failed to iterate for reweight: %v", err)
			c.Redirect(http.StatusFound, "/")
			return
		}

		var stock Stock
		doc.DataTo(&stock)

		// Reset previous recommendations and calculate score
		stock.Recommendation = ""
		if stock.IsBelowMA && stock.MA200 > 0 {
			score := stock.MA200 - stock.CurrentPrice
			totalScore += score
		}
		stocksToUpdate = append(stocksToUpdate, &stock)
	}

	// 2. Calculate and apply recommendations
	if totalScore > 0 {
		for _, stock := range stocksToUpdate {
			if stock.IsBelowMA && stock.MA200 > 0 {
				score := stock.MA200 - stock.CurrentPrice
				weight := score / totalScore
				investment := budget * weight
				stock.Recommendation = fmt.Sprintf("Invest €%.2f", investment)
			}
		}
	}

	// 3. Save all changes back to Firestore
	for _, stock := range stocksToUpdate {
		_, err := firestoreClient.Collection("portfolio").Doc(stock.Ticker).Set(ctx, stock)
		if err != nil {
			log.Printf("Failed to update stock %s for reweight: %v", stock.Ticker, err)
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
