package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const inventoryServiceURL = "http://localhost:8081"

var cb = NewCircuitBreaker(
	5,              // open after 5 consecutive failures
	2,              // close after 2 successes in half-open
	10*time.Second, // stay open for 10 seconds before half-open
)

type OrderRequest struct {
	ItemID   int `json:"item_id"`
	Quantity int `json:"quantity"`
}

type OrderResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func reserveInventory(itemID, quantity int) error {
	payload, _ := json.Marshal(map[string]int{
		"item_id":  itemID,
		"quantity": quantity,
	})

	resp, err := http.Post(
		inventoryServiceURL+"/inventory/reserve",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("inventory service error %d: %s", resp.StatusCode, body)
	}
	return nil
}

func validateOrder(req OrderRequest) error {
	if req.ItemID <= 0 {
		return fmt.Errorf("item_id must be a positive integer, got %d", req.ItemID)
	}
	if req.Quantity <= 0 {
		return fmt.Errorf("quantity must be a positive integer, got %d", req.Quantity)
	}
	if req.Quantity > 1000 {
		return fmt.Errorf("quantity exceeds maximum allowed (1000), got %d", req.Quantity)
	}
	return nil
}

func placeOrder(w http.ResponseWriter, r *http.Request) {
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Fail fast: reject invalid input before touching any downstream service
	if err := validateOrder(req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(OrderResponse{Status: "rejected", Message: err.Error()})
		return
	}

	err := cb.Call(func() error {
		return reserveInventory(req.ItemID, req.Quantity)
	})

	if err == ErrCircuitOpen {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(OrderResponse{
			Status:  "rejected",
			Message: "inventory service unavailable (circuit open)",
		})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(OrderResponse{
			Status:  "failed",
			Message: err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(OrderResponse{Status: "success"})
}

func healthz(w http.ResponseWriter, r *http.Request) {
	state := cb.State()
	stateStr := map[State]string{
		StateClosed:   "closed",
		StateOpen:     "open",
		StateHalfOpen: "half-open",
	}[state]
	fmt.Fprintf(w, `{"circuit_breaker":"%s"}`, stateStr)
}

func main() {
	http.HandleFunc("/order", placeOrder)
	http.HandleFunc("/healthz", healthz)

	port := ":8080"
	log.Printf("Order service listening on %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
