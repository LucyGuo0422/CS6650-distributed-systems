package store

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store provides S3-backed file storage
type S3Store struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
	region   string
}

// NewS3Store creates a new S3 store
func NewS3Store(client *s3.Client, bucket, region string) *S3Store {
	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 8 * 1024 * 1024 // 8MB parts — fewer API calls than 5MB
		u.Concurrency = 20           // 20 parallel part uploads
	})
	return &S3Store{
		client:   client,
		uploader: uploader,
		bucket:   bucket,
		region:   region,
	}
}

// UploadPhoto uploads a photo to S3
func (s *S3Store) UploadPhoto(photoID string, data io.Reader) error {
	key := fmt.Sprintf("photos/%s", photoID)

	_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   data,
	})
	if err != nil {
		return fmt.Errorf("failed to upload photo: %w", err)
	}

	return nil
}

// UploadPhotoToStaging uploads a photo to the staging location
func (s *S3Store) UploadPhotoToStaging(photoID string, data io.Reader) error {
	key := fmt.Sprintf("photos/pending/%s", photoID)

	_, err := s.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   data,
	})
	if err != nil {
		return fmt.Errorf("failed to upload photo to staging: %w", err)
	}

	return nil
}

// MovePhotoToFinal moves a photo from staging to final location
func (s *S3Store) MovePhotoToFinal(photoID string) error {
	stagingKey := fmt.Sprintf("photos/pending/%s", photoID)
	finalKey := fmt.Sprintf("photos/%s", photoID)

	// Step 1: Copy from staging to final
	_, err := s.client.CopyObject(context.TODO(), &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(fmt.Sprintf("/%s/%s", s.bucket, stagingKey)),
		Key:        aws.String(finalKey),
	})
	if err != nil {
		// Check if staging file doesn't exist
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404") {
			return fmt.Errorf("staging file not found: %w", err)
		}
		return fmt.Errorf("failed to copy photo to final location: %w", err)
	}

	// Step 2: Delete staging file
	_, err = s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
	})
	if err != nil {
		// Log but don't fail - final file is already created
		log.Printf("WARN: Failed to delete staging file %s: %v", stagingKey, err)
	}

	return nil
}

// DeletePhoto deletes a photo from both staging and final locations
func (s *S3Store) DeletePhoto(photoID string) error {
	finalKey := fmt.Sprintf("photos/%s", photoID)
	stagingKey := fmt.Sprintf("photos/pending/%s", photoID)

	// Try deleting from final location
	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(finalKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete photo from final location: %w", err)
	}

	// Also try deleting from staging (ignore errors if not found)
	_, _ = s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(stagingKey),
	})

	return nil
}

// UploadPhotoMultipart uploads a photo using S3 multipart upload for parallel part uploads
func (s *S3Store) UploadPhotoMultipart(photoID string, data io.Reader) error {
	key := fmt.Sprintf("photos/%s", photoID)

	_, err := s.uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   data,
	})
	if err != nil {
		return fmt.Errorf("failed to multipart upload photo: %w", err)
	}

	return nil
}

// GetPhotoURL returns the public URL for a photo
func (s *S3Store) GetPhotoURL(photoID string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/photos/%s", s.bucket, s.region, photoID)
}
