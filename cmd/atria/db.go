package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"atria/internal/database"
)

var forceDrop bool

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

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbPingCmd, dbMigrateCmd, dbDropCmd)

	dbDropCmd.Flags().BoolVar(&forceDrop, "force", false, "Force database drop")
}
