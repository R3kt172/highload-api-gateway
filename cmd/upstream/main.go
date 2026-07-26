package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"service":  "demo-upstream",
			"method":   r.Method,
			"path":     r.URL.Path,
			"trace_id": r.Header.Get("X-Trace-ID"),
		})
	})
	log.Println("demo upstream listening on :9000")
	log.Fatal(http.ListenAndServe(":9000", nil))
}
