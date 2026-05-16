package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"atria/internal/database"
	"atria/internal/rss"
	"atria/internal/web"
	"database/sql"
)

var serverPort string

func startRSSWorker(ctx context.Context, db *sql.DB, interval time.Duration) {
	go func() {
		log.Printf("🔄 RSS background worker started (interval: %s)", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("RSS Worker: running scheduled fetch...")
				if err := rss.FetchAllActiveFeeds(ctx, db); err != nil {
					log.Printf("RSS Worker: fetch error: %v", err)
				}
			case <-ctx.Done():
				log.Println("RSS Worker: stopped.")
				return
			}
		}
	}()
}

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

		// 2. Start RSS background worker
		intervalMin := 20
		if s := os.Getenv("RSS_FETCH_INTERVAL"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				intervalMin = n
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		startRSSWorker(ctx, db, time.Duration(intervalMin)*time.Minute)

		// 3. Init web server
		srv := web.NewServer(db)
		router := srv.SetupRouter()

		// 4. Run
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
