## Project Overview

This project is a web application for portfolio balancing, developed in Go. It allows users to track their stock portfolio, analyze investment strategies, and visualize portfolio performance. The application is designed to be deployed as a containerized service on Google Cloud Run and uses Google Cloud Firestore as its database.

### Key Features

*   **Portfolio Management:** Users can add, delete, and update stocks in their portfolio.
*   **Stock Analysis:** The application fetches stock data from the Financial Modeling Prep (FMP) API to analyze stocks. It calculates the 200-day moving average (MA) and compares it to the current price to identify potentially undervalued stocks.
*   **Investment Strategy Simulation:** The core feature of the application is to compare two investment strategies:
    *   **200-Day MA Undervalued:** This strategy allocates a budget to stocks that are currently trading below their 200-day moving average.
    *   **Naive Proportional Allocation:** This strategy allocates the budget proportionally to the existing holdings in the portfolio.
    *   **EMA Trend Following:** A hypothetical strategy that logs a "sell" for the stock with the most negative 112-day EMA trend and "buys" for the two stocks with the most positive trends.
*   **Investment Logging:** The application logs all investment decisions for each of the three strategies into separate Firestore collections (`investment_logs`, `naive_strategy_logs`, `ema_logs`), allowing for detailed, side-by-side analysis and comparison.
*   **Portfolio History Visualization:** The application provides a chart to visualize the performance of both investment strategies over time.
*   **User Authentication:** A simple session-based authentication system is in place, with an "admin" user.

### Technical Details

*   **Language:** Go
*   **Web Framework:** Gin
*   **Database:** Google Cloud Firestore
*   **External APIs:** Financial Modeling Prep (FMP) API for stock data.
*   **Frontend:** The frontend is built with Go's native HTML templates. For data visualization, it uses **Chart.js**. For more details on the frontend implementation, see the `GEMINI.md` file in the `/templates` directory.
*   **Deployment:** The application is designed to be deployed as a containerized service using Docker, with a provided `Dockerfile` for building a production-ready image. It is intended to be run on Google Cloud Run.

### Project Structure

```
├── .gitignore
├── analysis.go         # Contains the logic for fetching and analyzing stock data from the FMP API.
├── Dockerfile          # Defines the Docker image for the application.
├── go.mod              # Go module definition file, listing dependencies.
├── go.sum              # Go module checksum file.
├── main.go             # The main application file, containing the web server, routing, and core application logic.
├── README.md           # The original README file for the project.
└── templates/
    ├── chart.tmpl.html # HTML template for the portfolio history chart.
    ├── index.tmpl.html # HTML template for the main portfolio page.
    ├── login.tmpl.html # HTML template for the login page.
    └── logs.tmpl.html  # HTML template for the investment logs page.
```

### How to Run

1.  **Set up Environment Variables:**
    *   `PROJECT_ID`: Your Google Cloud project ID.
    *   `FMP_API_KEY`: Your API key for the Financial Modeling Prep API.
    *   `ADMIN_PASSWORD`: The password for the "admin" user.
2.  **Run Locally:**
    ```bash
    go run .
    ```
3.  **Build and Run with Docker:**
    ```bash
    docker build -t portfolio-app .
    docker run -p 8080:8080 -e PROJECT_ID=<your-project-id> -e FMP_API_KEY=<your-fmp-api-key> -e ADMIN_PASSWORD=<your-admin-password> portfolio-app
    ```

### Development Principles

*   **Use Demonstrative Data First:** When implementing new features, especially on the frontend, always start with a simple, hardcoded, or "dummy" data sample to ensure the component works as expected. Once the component is verified with the sample data, then proceed to integrate it with the actual data from the application's backend. This approach isolates frontend development from backend data parsing and ensures a clear separation of concerns.
