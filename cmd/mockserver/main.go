package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type ServerInfo struct {
	Status  string `json:"status"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	RSS     string `json:"rss"`
	RSSAuth string `json:"rss_auth"`
	Article string `json:"article"`
}

func main() {
	portFlag := flag.Int("port", 0, "Port to run the mock server on (0 for random free port)")
	flag.Parse()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *portFlag))
	if err != nil {
		log.Fatalf("Failed to bind to port: %v", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/rss.xml", handleRSS(baseURL))
	mux.HandleFunc("/rss-auth.xml", handleRSSAuth(baseURL))
	mux.HandleFunc("/article/", handleArticle)

	info := ServerInfo{
		Status:  "running",
		Host:    "127.0.0.1",
		Port:    actualPort,
		RSS:     baseURL + "/rss.xml",
		RSSAuth: baseURL + "/rss-auth.xml",
		Article: baseURL + "/article/{id}",
	}

	jsonOut, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(jsonOut))

	server := &http.Server{Handler: mux}
	if err := server.Serve(listener); err != nil {
		fmt.Fprintf(os.Stderr, "Server crashed: %v\n", err)
	}
}

func handleRSS(baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")

		now := time.Now()
		items := ""

		for i := 1; i <= 45; i++ {
			pubDate := now.Add(-time.Duration(i) * time.Hour).Format(time.RFC1123Z)
			itemURL := fmt.Sprintf("%s/article/%d", baseURL, i)

			item := fmt.Sprintf(`
		<item>
			<title>Mock Article #%d: The Future of Testing</title>
			<link>%s</link>
			<description>This is a short triage description for article %d.</description>
			<content:encoded><![CDATA[<p>This is the full inline content for article %d. It should be visible when you click "Read inline ↓".</p>]]></content:encoded>
			<pubDate>%s</pubDate>
			<guid>%s</guid>
		</item>`, i, itemURL, i, i, pubDate, itemURL)
			items += item
		}

		feed := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
	<title>Atria Local Mock Feed</title>
	<link>%s</link>
	<description>Deterministic test feed for Atria Read-it-Later</description>
	%s
</channel>
</rss>`, baseURL, items)

		w.Write([]byte(feed))
	}
}

func handleRSSAuth(baseURL string) http.HandlerFunc {
	rssHandler := handleRSS(baseURL)
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		rssHandler(w, r)
	}
}

func handleArticle(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/article/")
	id, _ := strconv.Atoi(idStr)

	w.Header().Set("Content-Type", "text/html")

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Mock Article #%d: The Future of Testing</title>
	<meta property="og:site_name" content="AtriaMockDomain" />
	<style>
		body { font-family: sans-serif; }
		.ad-banner { background: #ffcccc; padding: 20px; text-align: center; border: 2px dashed red; margin: 10px 0; }
		.cookie-popup { position: fixed; bottom: 0; background: #000; color: #fff; width: 100%%; padding: 10px; }
		.social-share { display: flex; gap: 10px; list-style: none; }
		nav { background: #eee; padding: 10px; }
		aside { float: right; width: 200px; background: #f9f9f9; padding: 15px; }
	</style>
	<script>
		console.log("Tracking pixel loaded.");
		// Some fake inline script noise
		function toggleNav() { document.getElementById('menu').style.display = 'block'; }
	</script>
</head>
<body>
	<div class="cookie-popup">We use cookies. <button>Accept all</button></div>

	<nav id="menu">
		<ul>
			<li><a href="/">Home</a></li>
			<li><a href="/category/tech">Tech</a></li>
			<li><a href="/about">About Us</a></li>
		</ul>
	</nav>

	<div class="ad-banner">
		BUY ONE GET ONE FREE! CLOUD HOSTING SALE!
	</div>

	<aside>
		<h3>Trending Now</h3>
		<ul>
			<li><a href="/article/99">Why you need 128GB of RAM</a></li>
			<li><a href="/article/100">Top 10 JS Frameworks this week</a></li>
		</ul>
		<div class="ad-banner">Side Ad! Buy now!</div>
	</aside>

	<main>
		<article>
			<header>
				<h1>Mock Article #%d - Full Content</h1>
				<p class="author-meta">By <strong>Testy McTestface</strong> | Published on April 6, 2026</p>
				<ul class="social-share">
					<li><a href="#">Share on Twitter</a></li>
					<li><a href="#">Share on Facebook</a></li>
				</ul>
			</header>

			<div class="article-body">
				<p>This is a detailed article designed to test the <strong>go-readability</strong> parser in Go. The parser must extract this core content while ignoring the navigation, sidebar, cookie banner, and advertisements.</p>

				<h2>Testing Lazy Loading Fix</h2>
				<p>The image below uses a fake base64 src and a real data-src. Atria's regex should fix this automatically!</p>

				<figure>
					<img src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMSIgaGVpZ2h0PSIxIiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciPjwvc3ZnPg==" data-src="https://picsum.photos/seed/%d/400/200" alt="Lazy loaded image" />
					<figcaption>This image should be fixed by Atria regex.</figcaption>
				</figure>

				<div class="ad-banner">Mid-article annoying ad subscription box. Enter email: <input type="email"></div>

				<h2>Testing typography</h2>
				<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Suspendisse varius enim in eros elementum tristique.</p>
				<ul>
					<li>Important point 1</li>
					<li>Important point 2</li>
				</ul>
			</div>
		</article>
	</main>

	<footer>
		<p>&copy; 2026 Atria Mock News Corp. All rights reserved.</p>
		<ul><li>Privacy Policy</li><li>Terms of Service</li></ul>
	</footer>
</body>
</html>`, id, id, id)

	w.Write([]byte(html))
}
