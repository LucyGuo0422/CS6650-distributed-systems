package common

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// RegisterInternalHandlers registers /internal/set and /internal/get on the given mux.
// These are used for inter-node communication (Leader→Follower or Coordinator→Peer).
func RegisterInternalHandlers(mux *http.ServeMux, store *Store) {
	mux.HandleFunc("/internal/set", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}
		value := r.URL.Query().Get("value")
		versionStr := r.URL.Query().Get("version")
		version, err := strconv.ParseInt(versionStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid version", http.StatusBadRequest)
			return
		}

		// Follower/peer sleeps 100ms before writing
		time.Sleep(100 * time.Millisecond)

		store.SetWithVersion(key, value, version)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/internal/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Query().Get("key")

		// Follower sleeps 50ms before responding to Leader reads
		time.Sleep(50 * time.Millisecond)

		entry, ok := store.Get(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	})
}
