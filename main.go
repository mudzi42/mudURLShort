package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Configuration struct to hold settings
type Config struct {
	DBPath       string // Path to the SQLite database file
	ServerPort   string // Port on which the server will listen
	ShortURLBase string // Base URL for generated short URLs
}

func main() {
	// Seed the random number generator for generating short codes
	source := rand.NewSource(time.Now().UnixNano())
	rand.New(source)

	// Load configuration from environment variables or a config file
	config := loadConfig()

	// Open a connection to the SQLite database
	db, err := sql.Open("sqlite3", config.DBPath)
	if err != nil {
		log.Fatal("Error opening database:", err)
	}
	defer db.Close() // Ensure the database connection is closed when the program exits

	// Create the 'urls' table if it doesn't exist
	createTable(db)

	// Set up a connection pool to manage database connections efficiently
	db.SetMaxOpenConns(10) // Adjust the number of connections as needed

	// Handle requests to the root path ("/")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexHandler(w, r, db, config)
	})

	// Handle requests to the "/shorten" path for creating short URLs
	http.HandleFunc("/shorten", func(w http.ResponseWriter, r *http.Request) {
		shortenHandler(w, r, db, config)
	})

	// Start the server and listen on the configured port
	fmt.Printf("Server listening on port %s\n", config.ServerPort)
	log.Fatal(http.ListenAndServe(":"+config.ServerPort, nil))
}

// loadConfig loads configuration settings from environment variables or a config file
func loadConfig() Config {
	// In a production environment, you'd likely use a library like Viper or envconfig
	// to load configuration from environment variables or a config file.
	// For this example, we'll just hardcode some values.
	return Config{
		DBPath:       "./urls.db",
		ServerPort:   "8080",
		ShortURLBase: "http://short.url/",
	}
}

// indexHandler handles requests to the root path and renders the index.html template
func indexHandler(w http.ResponseWriter, r *http.Request, db *sql.DB, config Config) {
	tmpl := template.Must(template.ParseFiles("index.html"))
	err := tmpl.Execute(w, nil)
	if err != nil {
		log.Println("Error executing template:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// shortenHandler handles requests to create short URLs
func shortenHandler(w http.ResponseWriter, r *http.Request, db *sql.DB, config Config) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	longURL := r.FormValue("long_url")

	// Create a context with a timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var shortCode string
	for i := 0; i < 10; i++ { // Try generating a short code up to 10 times
		shortCode = generateShortCode(6)
		if !shortCodeExists(ctx, shortCode, db) { // Check if the generated short code is unique
			break
		}
	}

	if shortCodeExists(ctx, shortCode, db) {
		log.Println("Failed to generate a unique short code after 10 attempts")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Insert the new short URL into the database
	_, err := db.ExecContext(ctx, "INSERT INTO urls (short_code, long_url) VALUES (?, ?)", shortCode, longURL)
	if err != nil {
		log.Println("Error inserting into database:", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Construct the complete short URL and send it to the client
	shortURL := fmt.Sprintf("%s%s", config.ShortURLBase, shortCode)
	fmt.Fprintf(w, "Short URL: %s", shortURL)
}

// generateShortCode generates a random short code of the specified length
func generateShortCode(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var shortCode []byte
	for i := 0; i < length; i++ {
		shortCode = append(shortCode, charset[rand.Intn(len(charset))])
	}
	return string(shortCode)
}

// createTable creates the 'urls' table in the database if it doesn't exist
func createTable(db *sql.DB) {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS urls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            short_code TEXT UNIQUE NOT NULL,
            long_url TEXT NOT NULL
        )
    `)
	if err != nil {
		log.Fatal("Error creating table:", err)
	}
}

// shortCodeExists checks if a short code already exists in the database
func shortCodeExists(ctx context.Context, shortCode string, db *sql.DB) bool {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = ?)", shortCode).Scan(&exists)
	if err != nil {
		log.Println("Error checking short code existence:", err)
		return true // Assume it exists in case of an error
	}
	return exists
}
