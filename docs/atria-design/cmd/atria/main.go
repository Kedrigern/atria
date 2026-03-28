package main

import (
	"html/template"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	mux.HandleFunc("GET /{$}", makeHandler("home.html"))
	mux.HandleFunc("GET /rss", makeHandler("rss.html"))
	mux.HandleFunc("GET /read", makeHandler("read_list.html"))
	mux.HandleFunc("GET /notes", makeHandler("note_list.html"))
	mux.HandleFunc("GET /notes/detail", makeHandler("note_detail.html")) // Ukázka detailu
	mux.HandleFunc("GET /tables", makeHandler("table_list.html"))
	mux.HandleFunc("GET /settings", makeHandler("settings.html"))
	mux.HandleFunc("GET /profile", makeHandler("profile.html"))

	log.Println("🚀 Atria startuje na http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func makeHandler(tmplName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{"UnreadCount": 14}

		var t *template.Template
		var err error

		if r.Header.Get("HX-Request") == "true" {
			t, err = template.ParseFiles("templates/pages/" + tmplName)
			if err == nil {
				err = t.ExecuteTemplate(w, "content", data)
			}
		} else {
			t, err = template.ParseFiles("templates/base.html", "templates/pages/"+tmplName)
			if err == nil {
				err = t.ExecuteTemplate(w, "base.html", data)
			}
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
