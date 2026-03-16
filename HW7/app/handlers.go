package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"
)

// paymentSlot is a buffered channel of size 1 used as a semaphore.
// Send to acquire (blocks when full), receive to release.
// Only one payment can be processed at a time, capping throughput at ~0.33/s.
var paymentSlot = make(chan struct{}, 1)

// Order represents an incoming order request.
type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"` // pending | processing | completed
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

// Item is a line item within an Order.
type Item struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// SyncOrder handles POST /orders/sync.
// Blocks until the payment semaphore is acquired, simulates 3s processing, then responds 200 OK.
func SyncOrder(w http.ResponseWriter, r *http.Request) {
	var order Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	order.OrderID = uuid.New().String()
	order.Status = "pending"
	order.CreatedAt = time.Now()

	// Acquire the payment slot (blocks if another payment is in progress)
	paymentSlot <- struct{}{}
	order.Status = "processing"
	time.Sleep(3 * time.Second) // simulate payment processing
	order.Status = "completed"
	<-paymentSlot // release the slot

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(order)
}

// AsyncOrder returns an HTTP handler that publishes the order to SNS and immediately
// responds 202 Accepted. Actual payment processing happens asynchronously via SQS workers.
func AsyncOrder(snsClient *sns.Client, topicARN string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var order Order
		if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		order.OrderID = uuid.New().String()
		order.Status = "pending"
		order.CreatedAt = time.Now()

		payload, err := json.Marshal(order)
		if err != nil {
			http.Error(w, "failed to serialize order", http.StatusInternalServerError)
			return
		}

		_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
			TopicArn: aws.String(topicARN),
			Message:  aws.String(string(payload)),
		})
		if err != nil {
			http.Error(w, "failed to publish order", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted) // 202: accepted for async processing
		json.NewEncoder(w).Encode(order)
	}
}
