package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
)

// CronExpression represents a saved cron expression
type CronExpression struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Expression  string    `json:"expression"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ConvertRequest is the request body for converting a cron expression
type ConvertRequest struct {
	Expression string `json:"expression"`
}

// ConvertResponse is the response for a converted cron expression
type ConvertResponse struct {
	Description    string   `json:"description"`
	NextExecutions []string `json:"nextExecutions"`
}

var db *sql.DB

// Prometheus metrics
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by endpoint and status",
		},
		[]string{"endpoint", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	cronExpressionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cron_expressions_total",
			Help: "Total number of cron expressions stored",
		},
	)

	dbConnectionErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "db_connection_errors_total",
			Help: "Total number of database connection errors",
		},
	)

	invalidCronExpressions = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "invalid_cron_expressions_total",
			Help: "Total number of invalid cron expressions submitted",
		},
	)
)

func main() {

	logFile, err := os.OpenFile("cronops.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		os.Exit(1)
	}

	// Set log output to both stdout and file
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	// Load environment variables
	err = godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file")
	}

	// Connect to database
	initDB()

	// Create router
	r := mux.NewRouter()

	// Define routes with metrics middleware
	r.HandleFunc("/api/convert", metricMiddleware("/api/convert", convertCronHandler)).Methods("POST")
	r.HandleFunc("/api/expressions", metricMiddleware("/api/expressions", getExpressionsHandler)).Methods("GET")
	r.HandleFunc("/api/expressions", metricMiddleware("/api/expressions", createExpressionHandler)).Methods("POST")
	r.HandleFunc("/api/expressions/{id}", metricMiddleware("/api/expressions/{id}", getExpressionHandler)).Methods("GET")
	r.HandleFunc("/api/expressions/{id}", metricMiddleware("/api/expressions/{id}", updateExpressionHandler)).Methods("PUT")
	r.HandleFunc("/api/expressions/{id}", metricMiddleware("/api/expressions/{id}", deleteExpressionHandler)).Methods("DELETE")

	// Add Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// Serve static files
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static")))

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("Prometheus metrics available at /metrics")
	log.Fatal(http.ListenAndServe(":"+port, r))
}

// Middleware to record metrics for each request
func metricMiddleware(endpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a custom response writer to capture the status code
		crw := newCustomResponseWriter(w)

		// Call the next handler
		next(crw, r)

		// Record metrics after request is processed
		duration := time.Since(start).Seconds()
		httpRequestDuration.WithLabelValues(endpoint).Observe(duration)
		httpRequestsTotal.WithLabelValues(endpoint, fmt.Sprintf("%d", crw.statusCode)).Inc()
	}
}

// Custom response writer to capture status code
type customResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newCustomResponseWriter(w http.ResponseWriter) *customResponseWriter {
	return &customResponseWriter{w, http.StatusOK}
}

func (crw *customResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
	crw.ResponseWriter.WriteHeader(code)
}

func initDB() {
	var err error

	// Get database connection details from environment variables
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	// Set defaults if not provided
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	if user == "" {
		user = "postgres"
	}
	if dbname == "" {
		dbname = "cronconverter"
	}

	// Construct the connection string
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbname)

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		dbConnectionErrors.Inc()
		log.Fatal(err)
	}

	err = db.Ping()
	if err != nil {
		dbConnectionErrors.Inc()
		log.Fatal(err)
	}

	// Create table if not exists
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS cron_expressions (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            expression VARCHAR(255) NOT NULL,
            description TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
	if err != nil {
		log.Fatal(err)
	}

	// Count existing expressions for initial metric
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM cron_expressions").Scan(&count)
	if err == nil && count > 0 {
		cronExpressionsTotal.Add(float64(count))
	}

	log.Println("Database connected successfully")
}

func convertCronHandler(w http.ResponseWriter, r *http.Request) {
	var req ConvertRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err = parser.Parse(req.Expression)
	if err != nil {
		invalidCronExpressions.Inc()
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Generate human readable description
	description := generateDescription(req.Expression)

	// Calculate next execution times
	nextExecutions := calculateNextExecutions(req.Expression, 5)

	response := ConvertResponse{
		Description:    description,
		NextExecutions: nextExecutions,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getExpressionsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id, name, expression, description, created_at, updated_at 
		FROM cron_expressions 
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	expressions := []CronExpression{}
	for rows.Next() {
		var exp CronExpression
		err := rows.Scan(&exp.ID, &exp.Name, &exp.Expression, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		expressions = append(expressions, exp)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expressions)
}

func createExpressionHandler(w http.ResponseWriter, r *http.Request) {
	var exp CronExpression
	err := json.NewDecoder(r.Body).Decode(&exp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err = parser.Parse(exp.Expression)
	if err != nil {
		invalidCronExpressions.Inc()
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Insert into database
	now := time.Now()
	err = db.QueryRow(`
		INSERT INTO cron_expressions (name, expression, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, exp.Name, exp.Expression, exp.Description, now, now).Scan(&exp.ID, &exp.CreatedAt, &exp.UpdatedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Increment the counter for expressions
	cronExpressionsTotal.Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(exp)
}

func getExpressionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var exp CronExpression
	err := db.QueryRow(`
		SELECT id, name, expression, description, created_at, updated_at 
		FROM cron_expressions 
		WHERE id = $1
	`, id).Scan(&exp.ID, &exp.Name, &exp.Expression, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Expression not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exp)
}

func updateExpressionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var exp CronExpression
	err := json.NewDecoder(r.Body).Decode(&exp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err = parser.Parse(exp.Expression)
	if err != nil {
		invalidCronExpressions.Inc()
		http.Error(w, "Invalid cron expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update in database
	now := time.Now()
	result, err := db.Exec(`
		UPDATE cron_expressions 
		SET name = $1, expression = $2, description = $3, updated_at = $4
		WHERE id = $5
	`, exp.Name, exp.Expression, exp.Description, now, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	// Get updated record
	err = db.QueryRow(`
		SELECT id, name, expression, description, created_at, updated_at 
		FROM cron_expressions 
		WHERE id = $1
	`, id).Scan(&exp.ID, &exp.Name, &exp.Expression, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(exp)
}

func deleteExpressionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := db.Exec("DELETE FROM cron_expressions WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Expression not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Expression deleted successfully"})
}

func generateDescription(expression string) string {
	parts := strings.Fields(expression)
	if len(parts) != 5 {
		return "Invalid cron expression"
	}

	minute := parts[0]
	hour := parts[1]
	dayOfMonth := parts[2]
	month := parts[3]
	dayOfWeek := parts[4]

	description := "This cron expression will run "

	// Minutes
	minuteDesc := ""
	switch minute {
	case "*":
		minuteDesc = "every minute"
	case "*/1":
		minuteDesc = "every minute"
	case "0":
		minuteDesc = "at the start of each hour"
	case "*/5":
		minuteDesc = "every 5 minutes"
	case "*/10":
		minuteDesc = "every 10 minutes"
	case "*/15":
		minuteDesc = "every 15 minutes"
	case "*/30":
		minuteDesc = "every 30 minutes"
	default:
		if strings.Contains(minute, ",") {
			minuteDesc = fmt.Sprintf("at minutes %s", minute)
		} else if strings.Contains(minute, "-") {
			minuteDesc = fmt.Sprintf("every minute from %s", minute)
		} else if strings.Contains(minute, "/") {
			parts := strings.Split(minute, "/")
			if len(parts) == 2 {
				minuteDesc = fmt.Sprintf("every %s minute(s)", parts[1])
			}
		} else {
			minuteDesc = fmt.Sprintf("at minute %s", minute)
		}
	}

	// Hours
	hourDesc := ""
	switch hour {
	case "*":
		hourDesc = "every hour"
	case "*/1":
		hourDesc = "every hour"
	case "0":
		hourDesc = "at midnight"
	case "12":
		hourDesc = "at noon"
	default:
		if strings.Contains(hour, ",") {
			hourDesc = fmt.Sprintf("at hours %s", hour)
		} else if strings.Contains(hour, "-") {
			hourDesc = fmt.Sprintf("every hour from %s", hour)
		} else if strings.Contains(hour, "/") {
			parts := strings.Split(hour, "/")
			if len(parts) == 2 {
				hourDesc = fmt.Sprintf("every %s hour(s)", parts[1])
			}
		} else {
			hourDesc = fmt.Sprintf("at %s:00", hour)
		}
	}

	// Day of month
	domDesc := ""
	switch dayOfMonth {
	case "*":
		domDesc = "every day of the month"
	case "1":
		domDesc = "on the 1st of the month"
	case "2":
		domDesc = "on the 2nd of the month"
	case "3":
		domDesc = "on the 3rd of the month"
	case "L":
		domDesc = "on the last day of the month"
	default:
		if strings.Contains(dayOfMonth, ",") {
			domDesc = fmt.Sprintf("on days %s of the month", dayOfMonth)
		} else if strings.Contains(dayOfMonth, "-") {
			domDesc = fmt.Sprintf("on days %s of the month", dayOfMonth)
		} else if strings.Contains(dayOfMonth, "/") {
			parts := strings.Split(dayOfMonth, "/")
			if len(parts) == 2 {
				domDesc = fmt.Sprintf("every %s day(s) of the month", parts[1])
			}
		} else {
			suffix := "th"
			if dayOfMonth == "1" || dayOfMonth == "21" || dayOfMonth == "31" {
				suffix = "st"
			} else if dayOfMonth == "2" || dayOfMonth == "22" {
				suffix = "nd"
			} else if dayOfMonth == "3" || dayOfMonth == "23" {
				suffix = "rd"
			}
			domDesc = fmt.Sprintf("on the %s%s of the month", dayOfMonth, suffix)
		}
	}

	// Month
	monthDesc := ""
	monthNames := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	switch month {
	case "*":
		monthDesc = "every month"
	default:
		if strings.Contains(month, ",") {
			parts := strings.Split(month, ",")
			months := []string{}
			for _, m := range parts {
				if i, err := fmt.Sscanf(m, "%d", new(int)); err == nil && i > 0 && i <= 12 {
					months = append(months, monthNames[i])
				} else {
					months = append(months, m)
				}
			}
			monthDesc = fmt.Sprintf("in %s", strings.Join(months, ", "))
		} else if strings.Contains(month, "-") {
			parts := strings.Split(month, "-")
			if len(parts) == 2 {
				start, end := "", ""
				if i, err := fmt.Sscanf(parts[0], "%d", new(int)); err == nil && i > 0 && i <= 12 {
					start = monthNames[i]
				} else {
					start = parts[0]
				}
				if i, err := fmt.Sscanf(parts[1], "%d", new(int)); err == nil && i > 0 && i <= 12 {
					end = monthNames[i]
				} else {
					end = parts[1]
				}
				monthDesc = fmt.Sprintf("from %s to %s", start, end)
			}
		} else if i, err := fmt.Sscanf(month, "%d", new(int)); err == nil && i > 0 && i <= 12 {
			monthDesc = fmt.Sprintf("in %s", monthNames[i])
		} else {
			monthDesc = fmt.Sprintf("in month %s", month)
		}
	}

	// Day of week
	dowDesc := ""
	dowNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
	switch dayOfWeek {
	case "*":
		dowDesc = "on every day of the week"
	case "0", "7":
		dowDesc = "on Sundays"
	case "1":
		dowDesc = "on Mondays"
	case "2":
		dowDesc = "on Tuesdays"
	case "3":
		dowDesc = "on Wednesdays"
	case "4":
		dowDesc = "on Thursdays"
	case "5":
		dowDesc = "on Fridays"
	case "6":
		dowDesc = "on Saturdays"
	case "1-5":
		dowDesc = "on weekdays"
	case "0,6", "6,0", "6,7":
		dowDesc = "on weekends"
	default:
		if strings.Contains(dayOfWeek, ",") {
			parts := strings.Split(dayOfWeek, ",")
			days := []string{}
			for _, d := range parts {
				if i, err := fmt.Sscanf(d, "%d", new(int)); err == nil && i >= 0 && i <= 7 {
					idx := i
					if idx == 7 {
						idx = 0 // Both 0 and 7 represent Sunday
					}
					days = append(days, dowNames[idx])
				} else {
					days = append(days, d)
				}
			}
			dowDesc = fmt.Sprintf("on %s", strings.Join(days, ", "))
		} else if strings.Contains(dayOfWeek, "-") {
			parts := strings.Split(dayOfWeek, "-")
			if len(parts) == 2 {
				start, end := "", ""
				if i, err := fmt.Sscanf(parts[0], "%d", new(int)); err == nil && i >= 0 && i <= 7 {
					idx := i
					if idx == 7 {
						idx = 0
					}
					start = dowNames[idx]
				} else {
					start = parts[0]
				}
				if i, err := fmt.Sscanf(parts[1], "%d", new(int)); err == nil && i >= 0 && i <= 7 {
					idx := i
					if idx == 7 {
						idx = 0
					}
					end = dowNames[idx]
				} else {
					end = parts[1]
				}
				dowDesc = fmt.Sprintf("from %s to %s", start, end)
			}
		} else {
			dowDesc = fmt.Sprintf("on day %s of the week", dayOfWeek)
		}
	}

	// Special cases
	if minute == "0" && hour == "0" && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		return "This cron expression will run once per day at midnight."
	}

	if minute == "0" && hour == "0" && dayOfMonth == "*" && month == "*" && dayOfWeek == "0" {
		return "This cron expression will run at midnight on Sundays."
	}

	if minute == "0" && hour == "*" && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		return "This cron expression will run at the start of every hour."
	}

	// Combine descriptions
	if minute == "*" && hour == "*" {
		description += minuteDesc + " " + hourDesc
	} else if minute == "*" {
		description += "every minute " + hourDesc
	} else if hour == "*" {
		description += minuteDesc + " of every hour"
	} else {
		description += minuteDesc + " " + hourDesc
	}

	// Add day of month and month only if they're not wildcards
	if dayOfMonth != "*" {
		description += " " + domDesc
	}

	if month != "*" {
		description += " " + monthDesc
	}

	// Add day of week only if it's not a wildcard
	if dayOfWeek != "*" {
		description += " " + dowDesc
	}

	return description + "."
}

func calculateNextExecutions(expression string, count int) []string {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return []string{fmt.Sprintf("Error parsing cron expression: %s", err.Error())}
	}

	now := time.Now()
	next := schedule.Next(now)
	executions := []string{}

	for i := 0; i < count; i++ {
		executions = append(executions, next.Format("Mon Jan 2 2006 at 15:04:05"))
		next = schedule.Next(next)
	}

	return executions
}
