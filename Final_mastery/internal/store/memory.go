package store

import (
	"sync"
)

// Album represents an album entity
type Album struct {
	AlbumID     string `json:"album_id" dynamodbav:"album_id"`
	Title       string `json:"title" dynamodbav:"title"`
	Description string `json:"description" dynamodbav:"description"`
	Owner       string `json:"owner" dynamodbav:"owner"`
}

// Photo represents a photo entity
type Photo struct {
	PhotoID string `json:"photo_id" dynamodbav:"photo_id"`
	AlbumID string `json:"album_id" dynamodbav:"album_id"`
	Seq     int    `json:"seq" dynamodbav:"seq"`
	Status  string `json:"status" dynamodbav:"status"`
	URL     string `json:"url,omitempty" dynamodbav:"url,omitempty"`
}

// MemoryStore provides in-memory storage
type MemoryStore struct {
	albums      map[string]*Album
	photos      map[string]*Photo
	seqCounters map[string]int // album_id -> seq counter
	mu          sync.RWMutex
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		albums:      make(map[string]*Album),
		photos:      make(map[string]*Photo),
		seqCounters: make(map[string]int),
	}
}

// PutAlbum stores or updates an album
func (s *MemoryStore) PutAlbum(album *Album) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.albums[album.AlbumID] = album
	return nil
}

// GetAlbum retrieves an album by ID
func (s *MemoryStore) GetAlbum(albumID string) (*Album, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	album, ok := s.albums[albumID]
	if !ok {
		return nil, nil
	}
	return album, nil
}

// ListAlbums returns all albums
func (s *MemoryStore) ListAlbums() ([]*Album, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	albums := make([]*Album, 0, len(s.albums))
	for _, album := range s.albums {
		albums = append(albums, album)
	}
	return albums, nil
}

// IncrementSeq atomically increments and returns the seq for an album
func (s *MemoryStore) IncrementSeq(albumID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seqCounters[albumID]++
	return s.seqCounters[albumID]
}

// PutPhoto stores or updates a photo
func (s *MemoryStore) PutPhoto(photo *Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.photos[photo.PhotoID] = photo
	return nil
}

// GetPhoto retrieves a photo by ID
func (s *MemoryStore) GetPhoto(photoID string) (*Photo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	photo, ok := s.photos[photoID]
	if !ok {
		return nil, nil
	}
	return photo, nil
}

// DeletePhoto removes a photo
func (s *MemoryStore) DeletePhoto(photoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.photos, photoID)
	return nil
}
