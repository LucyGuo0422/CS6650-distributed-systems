//go:build ignore

// main_broken.go — the PROBLEMATIC version: no circuit breaker, no fail-fast.
// Used only to demonstrate cascading failures under load.
// Run with: go run main_broken.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

const inventoryURL = "http://localhost:8081"

type OrderReq struct {
	ItemID   int `json:"item_id"`
	Quantity int `json:"quantity"`
}

func brokenPlaceOrder(w http.ResponseWriter, r *http.Request) {
	var req OrderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// No validation, no circuit breaker — call inventory directly every time.
	payload, _ := json.Marshal(map[string]int{"item_id": req.ItemID, "quantity": req.Quantity})
	resp, err := http.Post(inventoryURL+"/inventory/reserve", "application/json", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, fmt.Sprintf("inventory error: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w, string(body), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"success"}`)
}

func main() {
	http.HandleFunc("/order", brokenPlaceOrder)
	log.Println("BROKEN order service on :8080 (no circuit breaker)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
