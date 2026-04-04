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

type config struct {
	mu        sync.RWMutex
	followers []string
	strategy  string
}

func (c *config) GetFollowers() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.followers))
	copy(result, c.followers)
	return result
}

func (c *config) SetFollowers(f []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.followers = f
}

func (c *config) GetStrategy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.strategy
}

func (c *config) SetStrategy(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.strategy = s
}

func main() {
	port := os.Getenv("NODE_PORT")
	if port == "" {
		port = "8080"
	}
	role := os.Getenv("ROLE") // "leader" or "follower"
	if role == "" {
		role = "leader"
	}

	store := common.NewStore()
	mux := http.NewServeMux()

	// All nodes get /local_read and internal endpoints
	mux.HandleFunc("/local_read", localReadHandler(store))
	common.RegisterInternalHandlers(mux, store)

	if role == "leader" {
		cfg := &config{
			followers: parseList(os.Getenv("FOLLOWERS")),
			strategy:  os.Getenv("STRATEGY"),
		}
		if cfg.strategy == "" {
			cfg.strategy = "w5r1"
		}
		log.Printf("Role=leader, strategy=%s, followers=%v", cfg.GetStrategy(), cfg.GetFollowers())

		// Runtime config endpoints
		mux.HandleFunc("/config/followers", configFollowersHandler(cfg))
		mux.HandleFunc("/config/strategy", configStrategyHandler(cfg))

		mux.HandleFunc("/set", leaderSetHandler(store, cfg))
		mux.HandleFunc("/get", leaderGetHandler(store, cfg))
	} else {
		log.Printf("Role=follower")
		mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
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
			version := store.Set(key, value)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(common.KVEntry{Value: value, Version: version})
		})
		mux.HandleFunc("/get", getHandler(store))
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func parseList(env string) []string {
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

// POST /config/followers with body: "http://ip1:8080,http://ip2:8080,..."
// GET /config/followers returns current followers
func configFollowersHandler(cfg *config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			newFollowers := parseList(string(body))
			cfg.SetFollowers(newFollowers)
			log.Printf("Followers updated to: %v", newFollowers)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"followers": newFollowers})
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"followers": cfg.GetFollowers()})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

// POST /config/strategy with body: "w5r1" or "w1r5" or "w3r3"
// GET /config/strategy returns current strategy
func configStrategyHandler(cfg *config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			s := strings.TrimSpace(string(body))
			cfg.SetStrategy(s)
			log.Printf("Strategy updated to: %s", s)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"strategy": s})
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"strategy": cfg.GetStrategy()})
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

// leaderSetHandler writes locally then replicates to followers based on strategy.
func leaderSetHandler(store *common.Store, cfg *config) http.HandlerFunc {
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

		followers := cfg.GetFollowers()
		strategy := cfg.GetStrategy()

		switch strategy {
		case "w5r1":
			if err := replicateSequential(followers, key, value, version, len(followers)); err != nil {
				http.Error(w, "replication failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		case "w1r5":
			go replicateSequential(followers, key, value, version, len(followers))
		case "w3r3":
			if err := replicateSequential(followers, key, value, version, 2); err != nil {
				http.Error(w, "replication failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(common.KVEntry{Value: value, Version: version})
	}
}

func replicateSequential(followers []string, key, value string, version int64, minAcks int) error {
	acks := 0
	var firstErr error

	for i, f := range followers {
		err := sendReplication(f, key, value, version)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("Replication to %s failed: %v", f, err)
		} else {
			acks++
		}

		if i < len(followers)-1 {
			time.Sleep(200 * time.Millisecond)
		}

		if acks >= minAcks && minAcks < len(followers) {
			remaining := followers[i+1:]
			go func() {
				for j, rf := range remaining {
					sendReplication(rf, key, value, version)
					if j < len(remaining)-1 {
						time.Sleep(200 * time.Millisecond)
					}
				}
			}()
			return nil
		}
	}

	if acks < minAcks {
		return fmt.Errorf("only %d/%d acks received: %v", acks, minAcks, firstErr)
	}
	return nil
}

func sendReplication(followerURL, key, value string, version int64) error {
	url := fmt.Sprintf("%s/internal/set?key=%s&value=%s&version=%d", followerURL, key, value, version)
	resp, err := http.Post(url, "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("follower returned %d", resp.StatusCode)
	}
	return nil
}

// leaderGetHandler reads based on strategy.
func leaderGetHandler(store *common.Store, cfg *config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		key := r.URL.Query().Get("key")

		followers := cfg.GetFollowers()
		strategy := cfg.GetStrategy()

		switch strategy {
		case "w5r1":
			entry, ok := store.Get(key)
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(entry)

		case "w1r5":
			best, found := readFromNodes(store, followers, key, len(followers))
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(best)

		case "w3r3":
			best, found := readFromNodes(store, followers, key, 2)
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(best)
		}
	}
}

func readFromNodes(store *common.Store, followers []string, key string, minFollowerResponses int) (common.KVEntry, bool) {
	localEntry, localOk := store.Get(key)

	type result struct {
		entry common.KVEntry
		ok    bool
	}
	ch := make(chan result, len(followers))

	for _, f := range followers {
		go func(followerURL string) {
			entry, ok := fetchFromFollower(followerURL, key)
			ch <- result{entry, ok}
		}(f)
	}

	best := localEntry
	bestFound := localOk
	received := 0

	for received < len(followers) {
		res := <-ch
		received++
		if res.ok && (!bestFound || res.entry.Version > best.Version) {
			best = res.entry
			bestFound = true
		}
		if received >= minFollowerResponses {
			break
		}
	}

	return best, bestFound
}

func fetchFromFollower(followerURL, key string) (common.KVEntry, bool) {
	url := fmt.Sprintf("%s/internal/get?key=%s", followerURL, key)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to read from %s: %v", followerURL, err)
		return common.KVEntry{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return common.KVEntry{}, false
	}
	var entry common.KVEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		log.Printf("Failed to decode from %s: %v", followerURL, err)
		return common.KVEntry{}, false
	}
	return entry, true
}
