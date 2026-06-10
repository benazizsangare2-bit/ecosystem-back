// package database

// import (
// 	"database/sql"
// 	"fmt"
// 	"log"
// 	"os"
// 	"strconv"

// 	"github.com/joho/godotenv"
// 	_ "github.com/lib/pq"
// )

// type Database struct {
// 	DB *sql.DB
// }

// var DB *sql.DB

// func ConnectDatabase() {
// 	err := godotenv.Load()
// 	if err != nil {
// 		log.Println("No .env file found, using environment variables")
// 	}

// 	// Check if running in Docker (DB_HOST environment variable is set by docker-compose)
// 	host := os.Getenv("DB_HOST")
// 	if host == "" {
// 		// Not in Docker, use localhost with .env values
// 		host = os.Getenv("HOST")
// 		if host == "" {
// 			host = "localhost"
// 		}
// 	}

// 	// Get port - handle both Docker and local
// 	portStr := os.Getenv("DB_PORT")
// 	var port int
// 	if portStr != "" {
// 		// Docker mode: use DB_PORT from docker-compose
// 		port, _ = strconv.Atoi(portStr)
// 	} else {
// 		// Local mode: use DB_PORT from .env file
// 		port, _ = strconv.Atoi(os.Getenv("DB_PORT"))
// 		if port == 0 {
// 			port = 5432 // Default PostgreSQL port
// 		}
// 	}

// 	user := os.Getenv("DB_USER")
// 	if user == "" {
// 		user = os.Getenv("USER")
// 		if user == "" {
// 			user = "postgres"
// 		}
// 	}

// 	password := os.Getenv("DB_PASSWORD")
// 	if password == "" {
// 		password = os.Getenv("PASSWORD")
// 		if password == "" {
// 			password = "postgres123"
// 		}
// 	}

// 	dbname := os.Getenv("DB_NAME")
// 	if dbname == "" {
// 		dbname = os.Getenv("DB_NAME")
// 		if dbname == "" {
// 			dbname = "ecosystem"
// 		}
// 	}

// 	psqlSetup := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
// 		host, port, user, password, dbname)

// 	db, errSql := sql.Open("postgres", psqlSetup)
// 	if errSql != nil {
// 		fmt.Println("There was an error when trying to connect to database", errSql)
// 		panic(errSql)
// 	} else {
// 		DB = db
// 		fmt.Println("Successfully connected to the database")
// 	}
// }

//	func (database *Database) InitDatabase() {
//		tableQueries := GetTableQueries()
//		for _, query := range tableQueries {
//			_, err := database.DB.Exec(query)
//			if err != nil {
//				log.Fatal(err)
//			}
//		}
//	}
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

type Database struct {
	DB *sql.DB
}

var DB *sql.DB

const SCHEMA_NAME = "environment_reports"

func ConnectDatabase() {
	// Check for Render DATABASE_URL first (production)
	databaseURL := os.Getenv("DATABASE_URL")
	
	if databaseURL != "" {
		// Production mode (Render)
		log.Println("Connecting using DATABASE_URL (production mode)")
		
		// First, connect without search_path to create schema
		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			log.Fatal("Failed to connect to database:", err)
		}
		
		// Create the schema if it doesn't exist
		_, err = db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", SCHEMA_NAME))
		if err != nil {
			log.Printf("Warning: Could not create schema: %v", err)
		} else {
			log.Printf("Schema '%s' ready", SCHEMA_NAME)
		}
		
		// Now reconnect with search_path set to the schema
		db.Close()
		
		// Add search_path to the connection string
		if strings.Contains(databaseURL, "?") {
			databaseURL = databaseURL + "&search_path=" + SCHEMA_NAME
		} else {
			databaseURL = databaseURL + "?search_path=" + SCHEMA_NAME
		}
		
		db, err = sql.Open("postgres", databaseURL)
		if err != nil {
			log.Fatal("Failed to connect to database with schema:", err)
		}
		
		DB = db
		fmt.Println("Successfully connected to production database")
		return
	}
	
	// Development mode (local or Docker)
	log.Println("No DATABASE_URL found, using development mode")
	
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = os.Getenv("HOST")
		if host == "" {
			host = "localhost"
		}
	}
	
	portStr := os.Getenv("DB_PORT")
	var port int
	if portStr != "" {
		port, _ = strconv.Atoi(portStr)
	} else {
		port = 5432
	}
	
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "postgres123"
	}
	
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "ecosystem"
	}
	
	psqlSetup := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	
	db, err := sql.Open("postgres", psqlSetup)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	
	DB = db
	fmt.Println("Successfully connected to development database")
}

func (database *Database) InitDatabase() {
	// Only run migrations in development or if specifically enabled
	if os.Getenv("RUN_MIGRATIONS") != "false" {
		// Set the search path for the current connection
		_, err := database.DB.Exec(fmt.Sprintf("SET search_path TO %s, public", SCHEMA_NAME))
		if err != nil {
			log.Printf("Warning: Could not set search_path: %v", err)
		}
		
		tableQueries := GetTableQueries()
		for _, query := range tableQueries {
			_, err := database.DB.Exec(query)
			if err != nil {
				// Ignore "already exists" errors
				if !strings.Contains(err.Error(), "already exists") &&
				   !strings.Contains(err.Error(), "duplicate key") {
					log.Printf("Migration warning: %v", err)
				}
			}
		}
		log.Println("Database migrations completed")
	}
}