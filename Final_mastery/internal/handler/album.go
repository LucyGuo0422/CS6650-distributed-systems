package handler

import (
	"album-store/internal/store"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AlbumStore interface for album storage operations
type AlbumStore interface {
	PutAlbum(album *store.Album) error
	GetAlbum(albumID string) (*store.Album, error)
	ListAlbums() ([]*store.Album, error)
}

// AlbumHandler handles album-related requests
type AlbumHandler struct {
	store AlbumStore
}

// NewAlbumHandler creates a new album handler
func NewAlbumHandler(store AlbumStore) *AlbumHandler {
	return &AlbumHandler{store: store}
}

// PutAlbum handles PUT /albums/:album_id
func (h *AlbumHandler) PutAlbum(c *gin.Context) {
	albumID := c.Param("album_id")

	var req struct {
		AlbumID     string `json:"album_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Owner       string `json:"owner"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Use album_id from URL path, not from body
	album := &store.Album{
		AlbumID:     albumID,
		Title:       req.Title,
		Description: req.Description,
		Owner:       req.Owner,
	}

	if err := h.store.PutAlbum(album); err != nil {
		log.Printf("ERROR: Failed to put album %s: %v", albumID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store album"})
		return
	}

	// Always 200 — spec accepts "200 or 201", saves a DynamoDB GetItem round-trip
	c.JSON(http.StatusOK, album)
}

// GetAlbum handles GET /albums/:album_id
func (h *AlbumHandler) GetAlbum(c *gin.Context) {
	albumID := c.Param("album_id")

	album, err := h.store.GetAlbum(albumID)
	if err != nil {
		log.Printf("ERROR: Failed to get album %s: %v", albumID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve album"})
		return
	}

	if album == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.JSON(http.StatusOK, album)
}

// ListAlbums handles GET /albums
func (h *AlbumHandler) ListAlbums(c *gin.Context) {
	albums, err := h.store.ListAlbums()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list albums"})
		return
	}

	if albums == nil {
		albums = []*store.Album{}
	}

	c.JSON(http.StatusOK, albums)
}
