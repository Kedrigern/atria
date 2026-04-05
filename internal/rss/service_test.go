package rss_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"atria/internal/core"
	"atria/internal/database" // needed for shared DeleteEntity
	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/users"

	"github.com/joho/godotenv"
)

func setupRSSDB(t *testing.T) (*sql.DB, *core.User) {
	// 1. Load environment variables
	_ = godotenv.Load("../../.env")
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	// 2. Initialize DB and CHECK FOR ERROR
	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test db: %v", err) // This prevents the nil pointer panic
	}

	// 3. Reset and Migrate
	if err := database.ResetDB(db); err != nil {
		t.Fatalf("Failed to reset db: %v", err)
	}
	if err := database.MigrateUp(db); err != nil {
		t.Fatalf("Failed to migrate db: %v", err)
	}

	// 4. Create test user
	user, err := users.CreateUser(context.Background(), db, "rss@test.local", "RSS Tester", "pass", core.RoleUser)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return db, user
}

func TestRSSLifecycle(t *testing.T) {
	db, user := setupRSSDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Setup Mock RSS Server that serves both the Feed and the Article content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			w.Header().Set("Content-Type", "application/rss+xml")
			// Crucial: Use absolute URL for the item link so readability can fetch it
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8" ?>
					<rss version="2.0">
					<channel>
						<title>Test Feed</title>
						<item>
							<title>Test Article</title>
							<link>http://%s/article</link>
							<guid>item-1</guid>
						</item>
					</channel>
					</rss>`, r.Host)
			return
		}

		if r.URL.Path == "/article" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><h1>Test Article</h1><p>Full content here.</p></body></html>`)
			return
		}
	}))
	defer server.Close()

	// 2. Add Feed (feed variable is now used below)
	feed, err := rss.CreateFeed(ctx, db, user.ID, "Test Feed", server.URL+"/feed")
	if err != nil {
		t.Fatalf("CreateFeed failed: %v", err)
	}

	// 3. Fetch Items
	err = rss.FetchAllActiveFeeds(ctx, db)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	// 4. Get item from triage (rss_to_read_view)
	items, err := rss.ListItemsToRead(ctx, db, user.ID, 100, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("Expected 1 item in triage, got %d", len(items))
	}
	itemID := items[0].ID

	// 5. SAVE: Convert RSS item to Article
	article, err := rss.SaveItemAsArticle(ctx, db, user.ID, itemID)
	if err != nil {
		t.Fatalf("SaveItemAsArticle failed: %v", err)
	}

	// 6. Verify Article exists in DB
	var exists bool
	err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM articles WHERE id = $1)", article.ID).Scan(&exists)
	if err != nil || !exists {
		t.Error("Article was not found in the database")
	}

	// 7. Verify Triage is now empty
	itemsAfter, _ := rss.ListItemsToRead(ctx, db, user.ID, 100, 0)
	if len(itemsAfter) != 0 {
		t.Errorf("Expected triage to be empty, got %d", len(itemsAfter))
	}

	// Cleanup using the stored feed variable to satisfy the IDE
	_ = notes.DeleteEntity(ctx, db, user.ID, feed.ID, false, true)
}
