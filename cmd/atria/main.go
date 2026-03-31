package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	// Ensure this import matches your module name in go.mod
	"atria/internal/database"
)

// Variable to store the state of the --force flag
var forceDrop bool

// ==========================================
// 1. ROOT COMMAND (`atria`)
// ==========================================
var rootCmd = &cobra.Command{
	Use:   "atria",
	Short: "Atria - Personal Mind Palace CLI",
	Long:  `Atria is a unified tool for managing your knowledge base, reading list, and RSS feeds.`,
}

// ==========================================
// 2. DATABASE COMMANDS (`atria db ...`)
// ==========================================
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "System and database administration",
}

var dbPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Verifies the connection to the PostgreSQL database",
	Run: func(cmd *cobra.Command, args []string) {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("ERROR: DATABASE_URL environment variable is not set")
		}

		db, err := database.InitDB(dsn)
		if err != nil {
			log.Fatalf("Ping failed: %v", err)
		}
		defer db.Close()

		fmt.Println("✅ PONG! Database connection is healthy.")
	},
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Applies all pending database migrations",
	Run: func(cmd *cobra.Command, args []string) {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("ERROR: DATABASE_URL environment variable is not set")
		}

		db, err := database.InitDB(dsn)
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()

		if err := database.MigrateUp(db); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}

		fmt.Println("✅ All migrations applied successfully.")
	},
}

var dbDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drops all tables and resets the database (requires --force)",
	Run: func(cmd *cobra.Command, args []string) {
		if !forceDrop {
			log.Fatal("ERROR: This is a destructive action. You must use the --force flag to drop the database.")
		}

		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("ERROR: DATABASE_URL environment variable is not set")
		}

		db, err := database.InitDB(dsn)
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()

		if err := database.ResetDB(db); err != nil {
			log.Fatalf("Drop failed: %v", err)
		}

		fmt.Println("✅ Database dropped. You can now run 'atria db migrate' again.")
	},
}

// ==========================================
// INITIALIZATION & MAIN
// ==========================================

func init() {
	// 1. Load variables from the .env file
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, relying on system environment variables.")
	}

	// 2. Build the command tree
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbPingCmd, dbMigrateCmd, dbDropCmd)

	// 3. Register flags
	// Tell Cobra that dbDropCmd accepts the --force flag and stores the result in forceDrop
	dbDropCmd.Flags().BoolVar(&forceDrop, "force", false, "Force database drop")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
