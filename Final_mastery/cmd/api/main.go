package main

import (
	"album-store/internal/handler"
	"album-store/internal/store"
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = "us-west-2"
	}
	albumsTable := os.Getenv("DYNAMODB_ALBUMS_TABLE")
	if albumsTable == "" {
		albumsTable = "albums"
	}
	photosTable := os.Getenv("DYNAMODB_PHOTOS_TABLE")
	if photosTable == "" {
		photosTable = "photos"
	}
	s3Bucket := os.Getenv("S3_BUCKET")

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
	s3Client := s3.NewFromConfig(cfg)

	// Initialize stores
	dynamoStore := store.NewDynamoDBStore(dynamoClient, albumsTable, photosTable)
	s3Store := store.NewS3Store(s3Client, s3Bucket, awsRegion)

	// Initialize handlers
	albumHandler := handler.NewAlbumHandler(dynamoStore)
	photoHandler := handler.NewPhotoHandler(dynamoStore, s3Store)

	// Release mode: no debug logging per request
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery()) // keep crash recovery, skip request logger

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Album endpoints
	router.PUT("/albums/:album_id", albumHandler.PutAlbum)
	router.GET("/albums/:album_id", albumHandler.GetAlbum)
	router.GET("/albums", albumHandler.ListAlbums)

	// Photo endpoints
	router.POST("/albums/:album_id/photos", photoHandler.PostPhoto)
	router.GET("/albums/:album_id/photos/:photo_id", photoHandler.GetPhoto)
	router.DELETE("/albums/:album_id/photos/:photo_id", photoHandler.DeletePhoto)

	// Configure HTTP server with explicit timeouts
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown on SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

