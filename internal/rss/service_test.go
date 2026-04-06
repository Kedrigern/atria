package rss_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"
	"atria/internal/testutil"
)

func TestRSSLifecycle(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
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

func TestE2EWorkflow(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// 2. Setup Mock Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed.xml" {
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")

			var items strings.Builder

			for i := 1; i <= 5; i++ {
				fmt.Fprintf(&items, `
					<item>
						<title>Article %d</title>
						<link>http://%s/article/%d</link>
						<guid>item-%d</guid>
					</item>`, i, r.Host, i, i)
			}
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8" ?><rss version="2.0"><channel><title>E2E Feed</title>%s</channel></rss>`, items.String())
			return
		}

		if strings.HasPrefix(r.URL.Path, "/article/") {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html>
				<head><title>E2E Test Article</title></head>
				<body>
					<nav class="menu">Ignore this nav because it is a menu.</nav>

					<article class="post-content">
						<h1>Real Content</h1>
						<p>This is the core text. It needs to be significantly longer so that the readability scoring algorithm actually recognizes it as the primary content of the page. If it is too short, the algorithm falls back to extracting the entire body. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>

						<img src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMSIgaGVpZ2h0PSIxIiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjwvc3ZnPg==" data-src="http://%s/real-image.jpg" />
					</article>

					<aside class="sidebar">Ignore this sidebar, it contains ads and related links.</aside>
				</body>
				</html>`, r.Host)
			return
		}
	}))
	defer server.Close()

	// --- SUB-TESTY ---

	t.Run("UpdateUserPreferences", func(t *testing.T) {
		prefs := core.DefaultPreferences()
		prefs.PaginationSize = 2 // Omezíme limit schválně na 2

		prefsJSON, err := json.Marshal(prefs)
		if err != nil {
			t.Fatalf("Failed to marshal preferences: %v", err)
		}

		_, err = db.ExecContext(ctx, "UPDATE users SET preferences = $1 WHERE id = $2", prefsJSON, user.ID)
		if err != nil {
			t.Fatalf("Failed to update user preferences in DB: %v", err)
		}
		user.Preferences = prefs
	})

	t.Run("AddAndFetchRSS", func(t *testing.T) {
		_, err := rss.CreateFeed(ctx, db, user.ID, "E2E Feed", server.URL+"/feed.xml")
		if err != nil {
			t.Fatalf("CreateFeed failed: %v", err)
		}

		err = rss.FetchAllActiveFeeds(ctx, db)
		if err != nil {
			t.Fatalf("Worker failed to fetch feeds: %v", err)
		}
	})

	var firstItemID string

	t.Run("VerifyRSSPagination", func(t *testing.T) {
		limit := user.Preferences.PaginationSize

		items, err := rss.ListItemsToRead(ctx, db, user.ID, limit, 0)
		if err != nil {
			t.Fatalf("Failed to list items: %v", err)
		}

		if len(items) != 2 {
			t.Errorf("Expected exactly %d items due to pagination, got %d", limit, len(items))
		}

		firstItemID = items[0].ID.String()
	})

	t.Run("ExtractManualArticle", func(t *testing.T) {
		if firstItemID == "" {
			t.Skip("Skipping because previous test failed")
		}

		// OPRAVA: Použití standardního ParseUUID s chycením chyby
		parsedItemID, err := core.ParseUUID(firstItemID)
		if err != nil {
			t.Fatalf("Failed to parse first item ID: %v", err)
		}

		// Konverze RSS na Article (uloží HTML)
		article, err := rss.SaveItemAsArticle(ctx, db, user.ID, parsedItemID)
		if err != nil {
			t.Fatalf("SaveItemAsArticle failed: %v", err)
		}

		var htmlContent string
		err = db.QueryRowContext(ctx, "SELECT html_content FROM articles WHERE id = $1", article.ID).Scan(&htmlContent)
		if err != nil {
			t.Fatalf("Failed to fetch article HTML: %v", err)
		}

		// 1. Ověříme, že Readability funguje
		if strings.Contains(htmlContent, "Ignore this nav") {
			t.Errorf("Readability failed: Navigation was not stripped")
		}

		// 2. Ověříme náš Regex fix pro obrázky
		if strings.Contains(htmlContent, "data:image/svg+xml") {
			t.Errorf("Lazy loading fix failed: Base64 dummy image is still present")
		}
		if !strings.Contains(htmlContent, `src="http://`+server.Listener.Addr().String()+`/real-image.jpg"`) {
			t.Errorf("Lazy loading fix failed: Real image URL was not promoted to src")
		}
	})
}
