package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"atria/internal/database"
	"atria/internal/rss"
	"atria/internal/web"
	"database/sql"
)

var (
	serverPort string
	serverHost string
)

// listenAddress builds the address to listen on from --host/--port or
// ATRIA_HOST/PORT. An empty host means all interfaces (the default); behind a
// reverse proxy on the same machine use 127.0.0.1 so the app cannot be
// reached around the proxy.
func listenAddress(flagHost, flagPort string) (string, error) {
	host := flagHost
	if host == "" {
		host = os.Getenv("ATRIA_HOST")
	}
	port := flagPort
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	if host != "" && host != "localhost" && net.ParseIP(host) == nil {
		return "", fmt.Errorf("invalid host %q: use an IP address such as 127.0.0.1 or ::1 (empty = all interfaces)", host)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %q", port)
	}
	return net.JoinHostPort(host, port), nil
}

func startRSSWorker(ctx context.Context, db *sql.DB, interval time.Duration) {
	go func() {
		log.Printf("🔄 RSS background worker started (interval: %s)", interval)
		fetch := func() {
			log.Println("RSS Worker: running scheduled fetch...")
			if err := rss.FetchAllActiveFeeds(ctx, db); err != nil {
				log.Printf("RSS Worker: fetch error: %v", err)
			}
		}
		retention := rss.RetentionFromEnv()
		prune := func() {
			n, err := rss.PruneReadItems(ctx, db, retention)
			if err != nil {
				log.Printf("RSS Worker: retention error: %v", err)
				return
			}
			if n > 0 {
				log.Printf("RSS Worker: cleared content of %d read items older than %d days", n, int(retention.Hours()/24))
			}
		}

		// Catch up on feeds that became due while the server was down instead
		// of waiting a full interval.
		fetch()
		prune()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		retentionTicker := time.NewTicker(24 * time.Hour)
		defer retentionTicker.Stop()
		for {
			select {
			case <-ticker.C:
				fetch()
			case <-retentionTicker.C:
				prune()
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
		if err := web.ValidateSecurityConfig(); err != nil {
			return err
		}
		addr, err := listenAddress(serverHost, serverPort)
		if err != nil {
			return err
		}

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
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		startRSSWorker(ctx, db, time.Duration(intervalMin)*time.Minute)

		// 3. Init web server
		srv := web.NewServer(db)
		router := srv.SetupRouter()

		// 4. Run
		httpServer := &http.Server{
			Addr:              addr,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			// Generous body/response timeouts: uploads, EPUB exports and manual
			// feed fetches run inside the request.
			ReadTimeout:  5 * time.Minute,
			WriteTimeout: 5 * time.Minute,
			IdleTimeout:  2 * time.Minute,
		}

		serverErr := make(chan error, 1)
		go func() {
			log.Printf("🚀 Starting Atria web server on %s", describeListen(addr))
			serverErr <- httpServer.ListenAndServe()
		}()

		select {
		case err := <-serverErr:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-ctx.Done():
		}

		log.Println("Shutting down web server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVarP(&serverPort, "port", "p", "", "Port to run the server on (overrides PORT)")
	serverCmd.Flags().StringVar(&serverHost, "host", "", "Address to bind, e.g. 127.0.0.1 (overrides ATRIA_HOST; default: all interfaces)")
}

// describeListen renders the listen address for the startup log.
func describeListen(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	if host == "" {
		return "all interfaces, port " + port
	}
	return "http://" + addr
}
