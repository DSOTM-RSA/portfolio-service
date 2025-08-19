## Templates Overview

This directory contains the HTML templates for the Go web application. The frontend is rendered using Go's native `html/template` package.

### `chart.tmpl.html`

This template is responsible for visualizing the portfolio performance data.

*   **Charting Library:** It uses **Chart.js** to render a line chart comparing the two investment strategies.
*   **Data Fetching:** The chart data is fetched dynamically from the `/api/portfolio-history` endpoint when the page loads.
*   **Loading Indicator:** A CSS-based loading spinner is displayed while the data is being fetched to provide feedback to the user.
*   **Axis Configuration:** The X-axis is a time scale configured to display labels for each week, providing a clear and consistent view of the data over time.

### `index.tmpl.html`

This is the main dashboard of the application. It displays:

*   The user's current portfolio of stocks.
*   Forms for adding, updating, and deleting stocks.
*   A form for searching for new stocks.
*   Buttons for analyzing the portfolio and allocating the budget.

### `login.tmpl.html`

A simple login page with a form for the username and password.

### `logs.tmpl.html`

This page displays the detailed logs of all investment decisions made by the application, grouped by investment batch.
