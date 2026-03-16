package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func handler(ctx context.Context, snsEvent events.SNSEvent) error {
	for _, record := range snsEvent.Records {
		var order Order
		if err := json.Unmarshal([]byte(record.SNS.Message), &order); err != nil {
			log.Printf("failed to parse order: %v", err)
			return err
		}

		log.Printf("Processing order %s for customer %d", order.OrderID, order.CustomerID)

		// Simulate 3-second payment processing (same bottleneck as Part II)
		time.Sleep(3 * time.Second)

		log.Printf("Completed order %s", order.OrderID)
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
