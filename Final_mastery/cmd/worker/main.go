package main

import (
	"album-store/internal/store"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func main() {
	// Load configuration from environment
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-west-2"
	}
	photosTable := os.Getenv("DYNAMODB_PHOTOS_TABLE")
	if photosTable == "" {
		photosTable = "photos"
	}
	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")
	if sqsQueueURL == "" {
		log.Fatal("SQS_QUEUE_URL is required")
	}

	workerConcurrency := 50
	if envConcurrency := os.Getenv("WORKER_CONCURRENCY"); envConcurrency != "" {
		if val, err := strconv.Atoi(envConcurrency); err == nil {
			workerConcurrency = val
		}
	}

	// Initialize AWS SDK with aggressive HTTP transport tuning
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(awsRegion),
		config.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        500,
				MaxIdleConnsPerHost: 500,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second,
				ForceAttemptHTTP2:   true,
			},
		}),
		config.WithRetryer(func() aws.Retryer {
			return retry.AddWithMaxAttempts(retry.NewStandard(), 3)
		}),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Initialize AWS SDK clients once at startup
	dynamoClient := dynamodb.NewFromConfig(cfg)
	sqsClient := sqs.NewFromConfig(cfg)
	s3Client := s3.NewFromConfig(cfg)

	// Initialize store
	dynamoStore := store.NewDynamoDBStore(dynamoClient, "", photosTable)

	// Initialize S3 store
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		log.Fatal("S3_BUCKET is required")
	}
	s3Store := store.NewS3Store(s3Client, s3Bucket, awsRegion)

	// Create semaphore for bounded concurrency
	sem := make(chan struct{}, workerConcurrency)

	log.Printf("Starting worker with concurrency=%d", workerConcurrency)

	// Main processing loop
	for {
		// Receive messages from SQS with long-polling
		result, err := sqsClient.ReceiveMessage(context.TODO(), &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(sqsQueueURL),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20, // Long-polling
			VisibilityTimeout:   60, // 60 seconds visibility timeout
		})
		if err != nil {
			log.Printf("Failed to receive messages: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Process each message in a goroutine with bounded concurrency
		for _, msg := range result.Messages {
			sem <- struct{}{} // Acquire semaphore slot

			go func(m types.Message) {
				defer func() { <-sem }() // Release semaphore slot

				processMessage(sqsClient, dynamoStore, s3Store, sqsQueueURL, m)
			}(msg)
		}
	}
}

func processMessage(sqsClient *sqs.Client, dynamoStore *store.DynamoDBStore, s3Store *store.S3Store, queueURL string, msg types.Message) {
	if msg.Body == nil {
		log.Printf("Received message with nil body")
		deleteMessage(sqsClient, queueURL, msg.ReceiptHandle)
		return
	}

	photoID := *msg.Body

	log.Printf("INFO: Moving photo %s from staging to final", photoID)
	start := time.Now()

	// Move from staging to final S3 location
	err := s3Store.MovePhotoToFinal(photoID)
	if err != nil {
		if strings.Contains(err.Error(), "staging file not found") {
			log.Printf("ERROR: Staging file not found for photo %s - marking as failed", photoID)
			dynamoStore.UpdatePhotoStatus(photoID, "failed")
			deleteMessage(sqsClient, queueURL, msg.ReceiptHandle)
			return
		}

		log.Printf("ERROR: Failed to move photo %s: %v", photoID, err)
		return
	}

	duration := time.Since(start)
	log.Printf("SUCCESS: Photo %s moved to final location in %.2fs", photoID, duration.Seconds())

	// Update photo status to completed
	err = dynamoStore.UpdatePhotoStatus(photoID, "completed")
	if err != nil {
		log.Printf("ERROR: Failed to update status for photo %s: %v", photoID, err)
		// Still delete message - photo is already moved to final location
		// Retrying would incorrectly mark as failed since staging file is gone
		deleteMessage(sqsClient, queueURL, msg.ReceiptHandle)
		return
	}

	// Success - delete the message from SQS
	deleteMessage(sqsClient, queueURL, msg.ReceiptHandle)
	log.Printf("Successfully processed photo %s", photoID)
}

func deleteMessage(sqsClient *sqs.Client, queueURL string, receiptHandle *string) {
	if receiptHandle == nil {
		return
	}

	_, err := sqsClient.DeleteMessage(context.TODO(), &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: receiptHandle,
	})
	if err != nil {
		log.Printf("Failed to delete message: %v", err)
	}
}
