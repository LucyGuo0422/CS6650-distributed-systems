package handler

import (
	"album-store/internal/store"
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const smallFileLimit = 10 << 20 // 10MB — PutObject for small, multipart for large

// PhotoStore interface for photo storage operations
type PhotoStore interface {
	IncrementSeq(albumID string) (int, error)
	PutPhoto(photo *store.Photo) error
	GetPhoto(photoID string) (*store.Photo, error)
	DeletePhoto(photoID string) error
	UpdatePhotoStatus(photoID, status string) error
}

// FileStore interface for file storage operations
type FileStore interface {
	UploadPhoto(photoID string, data io.Reader) error
	UploadPhotoMultipart(photoID string, data io.Reader) error
	DeletePhoto(photoID string) error
	GetPhotoURL(photoID string) string
}

// PhotoHandler handles photo-related requests
type PhotoHandler struct {
	photoStore    PhotoStore
	fileStore     FileStore
	activeUploads sync.Map // map[photoID]context.CancelFunc
}

// NewPhotoHandler creates a new photo handler
func NewPhotoHandler(photoStore PhotoStore, fileStore FileStore) *PhotoHandler {
	return &PhotoHandler{
		photoStore: photoStore,
		fileStore:  fileStore,
	}
}

// PostPhoto handles POST /albums/:album_id/photos
func (h *PhotoHandler) PostPhoto(c *gin.Context) {
	albumID := c.Param("album_id")

	// Streaming multipart reader — avoids ParseMultipartForm buffering/disk-spill
	reader, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	// Find the "photo" part
	var fileData []byte
	var tmpPath string

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read multipart"})
			return
		}
		if part.FormName() != "photo" {
			part.Close()
			continue
		}

		// Read up to smallFileLimit+1 to determine small vs large
		var buf bytes.Buffer
		n, readErr := io.CopyN(&buf, part, smallFileLimit+1)
		if readErr != nil && readErr != io.EOF {
			part.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
			return
		}

		if n <= smallFileLimit {
			// Small file: fits in memory
			fileData = buf.Bytes()
		} else {
			// Large file: spill to temp file — no OOM, fast sequential write
			tmpFile, err := os.CreateTemp("", "photo-upload-*")
			if err != nil {
				part.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create temp file"})
				return
			}
			if _, err := tmpFile.Write(buf.Bytes()); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				part.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write temp file"})
				return
			}
			buf.Reset() // free the 10MB buffer
			if _, err := io.Copy(tmpFile, part); err != nil {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				part.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
				return
			}
			tmpFile.Close()
			tmpPath = tmpFile.Name()
		}
		part.Close()
		break
	}

	if fileData == nil && tmpPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "photo field required"})
		return
	}

	// Step 1: Generate photo_id
	photoID := uuid.New().String()

	// Step 2: Atomically increment seq counter
	seq, err := h.photoStore.IncrementSeq(albumID)
	if err != nil {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate seq"})
		return
	}

	url := h.fileStore.GetPhotoURL(photoID)

	// Step 3: Write durable metadata to DynamoDB with status=processing
	photo := &store.Photo{
		PhotoID: photoID,
		AlbumID: albumID,
		Seq:     seq,
		Status:  "processing",
		URL:     url,
	}

	if err := h.photoStore.PutPhoto(photo); err != nil {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store photo"})
		return
	}

	// Step 4: Return 202 immediately — upload happens in background goroutine
	ctx, cancel := context.WithCancel(context.Background())
	h.activeUploads.Store(photoID, cancel)

	if tmpPath != "" {
		go h.uploadFromTempFile(ctx, photoID, tmpPath)
	} else {
		go h.uploadSmallPhotoAsync(ctx, photoID, fileData)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"photo_id": photoID,
		"seq":      seq,
		"status":   "processing",
	})
}

// uploadSmallPhotoAsync handles background small file upload to S3
func (h *PhotoHandler) uploadSmallPhotoAsync(ctx context.Context, photoID string, data []byte) {
	defer h.activeUploads.Delete(photoID)

	// Check if cancelled before starting upload
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Upload directly to final S3 location
	err := h.fileStore.UploadPhoto(photoID, bytes.NewReader(data))
	if err != nil {
		log.Printf("ERROR: Failed to upload photo %s: %v", photoID, err)
		h.photoStore.UpdatePhotoStatus(photoID, "failed")
		return
	}

	// Check if cancelled after upload
	select {
	case <-ctx.Done():
		h.fileStore.DeletePhoto(photoID)
		return
	default:
	}

	// Conditional update — won't create zombie if photo was deleted during upload
	if err := h.photoStore.UpdatePhotoStatus(photoID, "completed"); err != nil {
		log.Printf("WARN: Failed to mark photo %s completed (may have been deleted): %v", photoID, err)
		h.fileStore.DeletePhoto(photoID)
	}
}

// uploadFromTempFile handles background large file upload from temp file to S3
func (h *PhotoHandler) uploadFromTempFile(ctx context.Context, photoID, tmpPath string) {
	defer h.activeUploads.Delete(photoID)
	defer os.Remove(tmpPath)

	select {
	case <-ctx.Done():
		return
	default:
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		log.Printf("ERROR: Failed to open temp file for photo %s: %v", photoID, err)
		h.photoStore.UpdatePhotoStatus(photoID, "failed")
		return
	}
	defer f.Close()

	err = h.fileStore.UploadPhotoMultipart(photoID, f)
	if err != nil {
		log.Printf("ERROR: Failed to upload large photo %s: %v", photoID, err)
		h.photoStore.UpdatePhotoStatus(photoID, "failed")
		return
	}

	select {
	case <-ctx.Done():
		h.fileStore.DeletePhoto(photoID)
		return
	default:
	}

	if err := h.photoStore.UpdatePhotoStatus(photoID, "completed"); err != nil {
		log.Printf("WARN: Failed to mark photo %s completed (may have been deleted): %v", photoID, err)
		h.fileStore.DeletePhoto(photoID)
	}
}

// GetPhoto handles GET /albums/:album_id/photos/:photo_id
func (h *PhotoHandler) GetPhoto(c *gin.Context) {
	albumID := c.Param("album_id")
	photoID := c.Param("photo_id")

	photo, err := h.photoStore.GetPhoto(photoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve photo"})
		return
	}

	if photo == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// Validate that album_id matches
	if photo.AlbumID != albumID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, photo)
}

// DeletePhoto handles DELETE /albums/:album_id/photos/:photo_id
func (h *PhotoHandler) DeletePhoto(c *gin.Context) {
	albumID := c.Param("album_id")
	photoID := c.Param("photo_id")

	// Step 1: GetItem to fetch the photo record
	photo, err := h.photoStore.GetPhoto(photoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve photo"})
		return
	}

	if photo == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// Validate that album_id matches
	if photo.AlbumID != albumID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// Step 1.5: Cancel any active upload for this photo
	if cancel, ok := h.activeUploads.LoadAndDelete(photoID); ok {
		if cancelFunc, ok := cancel.(context.CancelFunc); ok {
			log.Printf("INFO: Cancelling active upload for photo %s", photoID)
			cancelFunc()
		}
	}

	// Step 2: DeleteObject from S3 (before DynamoDB)
	if err := h.fileStore.DeletePhoto(photoID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file"})
		return
	}

	// Step 3: DeleteItem from DynamoDB
	if err := h.photoStore.DeletePhoto(photoID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete photo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
