package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// numWorkers returns the goroutine pool size from NUM_WORKERS env var.
// Phase 5 scaling values: 1, 5, 20, 100, 180, 200
func numWorkers() int {
	if v := os.Getenv("NUM_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// snsEnvelope matches the JSON wrapper SNS places around published messages
// when delivering to an SQS queue.
type snsEnvelope struct {
	Message string `json:"Message"`
}

// StartProcessor begins the SQS polling loop.
// It long-polls for batches of up to 10 messages and dispatches each one to a
// bounded pool of numWorkers goroutines for processing.
func StartProcessor(sqsClient *sqs.Client, queueURL string) {
	n := numWorkers()
	log.Printf("processor starting with %d workers", n)
	sem := make(chan struct{}, n)

	for {
		result, err := sqsClient.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20, // long polling
		})
		if err != nil {
			log.Printf("sqs ReceiveMessage error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range result.Messages {
			sem <- struct{}{} // acquire worker slot
			go func(m sqstypes.Message) {
				defer func() { <-sem }() // release slot when done
				processMessage(sqsClient, queueURL, m)
			}(msg)
		}
	}
}

// processMessage unwraps the SNS envelope, simulates payment processing, then deletes the message.
func processMessage(sqsClient *sqs.Client, queueURL string, msg sqstypes.Message) {
	// Unwrap the SNS envelope
	var envelope snsEnvelope
	if err := json.Unmarshal([]byte(*msg.Body), &envelope); err != nil {
		log.Printf("failed to unmarshal SNS envelope: %v", err)
		return
	}

	var order Order
	if err := json.Unmarshal([]byte(envelope.Message), &order); err != nil {
		log.Printf("failed to unmarshal order: %v", err)
		return
	}

	log.Printf("processing order %s for customer %d", order.OrderID, order.CustomerID)
	order.Status = "processing"
	time.Sleep(3 * time.Second) // simulate payment processing
	order.Status = "completed"
	log.Printf("completed order %s", order.OrderID)

	_, err := sqsClient.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.Printf("failed to delete message for order %s: %v", order.OrderID, err)
	}
}
