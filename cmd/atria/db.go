package main

import (
	"fmt"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			return fmt.Errorf("DATABASE_URL environment variable is not set")
		}

		db, err := database.InitDB(dsn)
		if err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}
		defer db.Close()

		fmt.Println("✅ PONG! Database connection is healthy.")
		return nil
	},
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Applies all pending database migrations",
	RunE: func(cmd *cobra.Command, args []string) error {
		dsn := os.Getenv("DATABASE_URL")
		db, err := database.InitDB(dsn)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer db.Close()

		if err := database.MigrateUp(db); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		fmt.Println("✅ All migrations applied successfully.")
		return nil
	},
}

var dbDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drops all tables and resets the database (requires --force)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !forceDrop {
			return fmt.Errorf("this is a destructive action. You must use the --force flag to drop the database")
		}

		dsn := os.Getenv("DATABASE_URL")
		db, err := database.InitDB(dsn)
		if err != nil {
			return fmt.Errorf("connection failed: %w", err)
		}
		defer db.Close()

		if err := database.ResetDB(db); err != nil {
			return fmt.Errorf("drop failed: %w", err)
		}
		fmt.Println("✅ Database dropped. You can now run 'atria db migrate' again.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbPingCmd, dbMigrateCmd, dbDropCmd)

	dbDropCmd.Flags().BoolVar(&forceDrop, "force", false, "Force database drop")
}
