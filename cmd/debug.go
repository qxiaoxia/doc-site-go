package main

import (
	"fmt"
	"net/http"
	"strings"
)

func debugHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("DEBUG: Path=%s, Method=%s\n", r.URL.Path, r.Method)
	path := strings.TrimPrefix(r.URL.Path, "/api/docs/")
	fmt.Printf("DEBUG: Trimmed path=%s\n", path)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Path: %s, Trimmed: %s", r.URL.Path, path)
}

func init() {
	http.HandleFunc("/debug/", debugHandler)
}
