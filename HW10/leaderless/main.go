package main

import (
	"encoding/json"
	"fmt"
	"io"
	"kv-store/common"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type peerList struct {
	mu    sync.RWMutex
	peers []string
}

func (p *peerList) Get() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]string, len(p.peers))
	copy(result, p.peers)
	return result
}

func (p *peerList) Set(peers []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = peers
}

func main() {
	port := os.Getenv("NODE_PORT")
	if port == "" {
		port = "8080"
	}

	pl := &peerList{peers: parsePeers(os.Getenv("PEERS"))}
	log.Printf("Leaderless node starting, peers=%v", pl.Get())

	store := common.NewStore()
	mux := http.NewServeMux()

	// Internal endpoints (replication + local_read)
	common.RegisterInternalHandlers(mux, store)
	mux.HandleFunc("/local_read", localReadHandler(store))

	// Runtime config endpoint — update peers without redeploy
	mux.HandleFunc("/config/peers", configPeersHandler(pl))

	// Client-facing endpoints
	mux.HandleFunc("/set", setHandler(store, pl))
	mux.HandleFunc("/get", getHandler(store))

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func parsePeers(env string) []string {
	if env == "" {
		return nil
	}
	parts := strings.Split(env, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// POST /config/peers with body: "http://ip1:8080,http://ip2:8080,..."
// GET /config/peers returns current peers
func configPeersHandler(pl *peerList) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			newPeers := parsePeers(string(body))
			pl.Set(newPeers)
			log.Printf("Peers updated to: %v", newPeers)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": newPeers})
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"peers": pl.Get()})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func localReadHandler(store *common.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Query().Get("key")
		entry, ok := store.Get(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}
}

// setHandler: this node becomes the Write Coordinator.
// Write locally, then replicate sequentially to all peers (W=N).
func setHandler(store *common.Store, pl *peerList) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		// Write locally first
		version := store.Set(key, value)

		// Replicate sequentially to all peers, sleep 200ms after each
		peers := pl.Get()
		for i, peer := range peers {
			err := sendReplication(peer, key, value, version)
			if err != nil {
				log.Printf("Replication to %s failed: %v", peer, err)
				http.Error(w, "replication failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Sleep 200ms after each send (before next peer)
			if i < len(peers)-1 {
				time.Sleep(200 * time.Millisecond)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(common.KVEntry{Value: value, Version: version})
	}
}

func sendReplication(peerURL, key, value string, version int64) error {
	url := fmt.Sprintf("%s/internal/set?key=%s&value=%s&version=%d", peerURL, key, value, version)
	resp, err := http.Post(url, "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("peer returned %d", resp.StatusCode)
	}
	return nil
}

// getHandler: R=1, return local value only.
func getHandler(store *common.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Query().Get("key")
		entry, ok := store.Get(key)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}
}
