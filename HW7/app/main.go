package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("unable to load AWS config: %v", err)
	}

	snsClient := sns.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)

	snsTopicARN := os.Getenv("SNS_TOPIC_ARN")
	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/orders/sync", SyncOrder)
	http.HandleFunc("/orders/async", AsyncOrder(snsClient, snsTopicARN))

	go StartProcessor(sqsClient, sqsQueueURL)

	log.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
