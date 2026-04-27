package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	"atria/internal/database"
	"atria/internal/web"
)

var serverPort string

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the Atria web server",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Connect to DB
		dsn := os.Getenv("DATABASE_URL")
		db, err := database.InitDB(dsn)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer db.Close()

		// 2. Init web server
		srv := web.NewServer(db)
		router := srv.SetupRouter()

		// 3. Run
		port := serverPort
		if port == "" {
			port = os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
		}

		log.Printf("🚀 Starting Atria web server on http://localhost:%s", port)
		return router.Run(":" + port)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&serverPort, "port", "p", "", "Port to run the server on (overrides .env)")
}
