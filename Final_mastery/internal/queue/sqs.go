package queue

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSQueue provides SQS-backed queue operations
type SQSQueue struct {
	client   *sqs.Client
	queueURL string
}

// NewSQSQueue creates a new SQS queue client
func NewSQSQueue(client *sqs.Client, queueURL string) *SQSQueue {
	return &SQSQueue{
		client:   client,
		queueURL: queueURL,
	}
}

// SendPhotoMessage sends a photo processing message to the queue
func (q *SQSQueue) SendPhotoMessage(photoID string) error {
	_, err := q.client.SendMessage(context.TODO(), &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(photoID),
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}
