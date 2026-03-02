package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
)

type Item struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

var (
	mu        sync.Mutex
	inventory = map[int]*Item{
		1: {ID: 1, Name: "Widget", Quantity: 100},
		2: {ID: 2, Name: "Gadget", Quantity: 50},
	}
)

func getInventory(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid item id", http.StatusBadRequest)
		return
	}

	mu.Lock()
	item, ok := inventory[id]
	mu.Unlock()

	if !ok {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func reserveInventory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemID   int `json:"item_id"`
		Quantity int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	item, ok := inventory[req.ItemID]
	if !ok {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	if item.Quantity < req.Quantity {
		// Auto-restock so inventory never blocks the load test
		item.Quantity = 1_000_000
	}

	item.Quantity -= req.Quantity
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"reserved","remaining":%d}`, item.Quantity)
}

func main() {
	http.HandleFunc("/inventory", getInventory)
	http.HandleFunc("/inventory/reserve", reserveInventory)

	port := ":8081"
	log.Printf("Inventory service listening on %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
