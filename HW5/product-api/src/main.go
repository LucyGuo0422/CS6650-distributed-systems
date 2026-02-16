package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type Product struct {
	ProductID    int    `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int    `json:"category_id"`
	Weight       int    `json:"weight"`
	SomeOtherID  int    `json:"some_other_id"`
}

var (
	store = map[int]Product{}
	mu    sync.RWMutex
)

func sendJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func sendError(w http.ResponseWriter, code int, errCode, msg string) {
	sendJSON(w, code, map[string]string{"error": errCode, "message": msg})
}

func getIDFromPath(path string) (int, bool) {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	if len(seg) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(seg[1])
	return id, err == nil && id >= 1
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// recover from any panic -> 500
		defer func() {
			if err := recover(); err != nil {
				sendError(w, 500, "INTERNAL_ERROR", "internal server error")
			}
		}()

		id, ok := getIDFromPath(r.URL.Path)
		if !ok {
			sendError(w, 400, "INVALID_INPUT", "invalid or missing product ID")
			return
		}

		// POST /products/{id}/details
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/details") {
			var p Product
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				sendError(w, 400, "INVALID_INPUT", "invalid JSON body")
				return
			}
			// validate all required fields per spec
			switch {
			case p.ProductID < 1:
				sendError(w, 400, "INVALID_INPUT", "product_id must be >= 1")
				return
			case p.ProductID != id:
				sendError(w, 400, "INVALID_INPUT", "product_id in body must match URL")
				return
			case len(p.SKU) == 0 || len(p.SKU) > 100:
				sendError(w, 400, "INVALID_INPUT", "sku must be 1-100 characters")
				return
			case len(p.Manufacturer) == 0 || len(p.Manufacturer) > 200:
				sendError(w, 400, "INVALID_INPUT", "manufacturer must be 1-200 characters")
				return
			case p.CategoryID < 1:
				sendError(w, 400, "INVALID_INPUT", "category_id must be >= 1")
				return
			case p.Weight < 0:
				sendError(w, 400, "INVALID_INPUT", "weight must be >= 0")
				return
			case p.SomeOtherID < 1:
				sendError(w, 400, "INVALID_INPUT", "some_other_id must be >= 1")
				return
			}

			mu.Lock()
			store[id] = p
			mu.Unlock()
			w.WriteHeader(204)
			return
		}

		// GET /products/{id}
		if r.Method == "GET" && !strings.HasSuffix(r.URL.Path, "/details") {
			mu.RLock()
			p, exists := store[id]
			mu.RUnlock()
			if !exists {
				sendError(w, 404, "NOT_FOUND", "product not found")
				return
			}
			sendJSON(w, 200, p)
			return
		}

		sendError(w, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	})

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}