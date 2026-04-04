package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const (
	leaderURL    = "http://localhost:8080"
	follower1URL = "http://localhost:8081"
	follower2URL = "http://localhost:8082"
	follower3URL = "http://localhost:8083"
	follower4URL = "http://localhost:8084"
)

type kvEntry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

func setKey(url, key, value string) (kvEntry, int, error) {
	resp, err := http.Post(fmt.Sprintf("%s/set?key=%s&value=%s", url, key, value), "", nil)
	if err != nil {
		return kvEntry{}, 0, err
	}
	defer resp.Body.Close()
	var entry kvEntry
	if resp.StatusCode == http.StatusCreated {
		json.NewDecoder(resp.Body).Decode(&entry)
	}
	return entry, resp.StatusCode, nil
}

func getKey(url, key string) (kvEntry, int, error) {
	resp, err := http.Get(fmt.Sprintf("%s/get?key=%s", url, key))
	if err != nil {
		return kvEntry{}, 0, err
	}
	defer resp.Body.Close()
	var entry kvEntry
	if resp.StatusCode == http.StatusOK {
		json.NewDecoder(resp.Body).Decode(&entry)
	}
	return entry, resp.StatusCode, nil
}

func localRead(url, key string) (kvEntry, int, error) {
	resp, err := http.Get(fmt.Sprintf("%s/local_read?key=%s", url, key))
	if err != nil {
		return kvEntry{}, 0, err
	}
	defer resp.Body.Close()
	var entry kvEntry
	if resp.StatusCode == http.StatusOK {
		json.NewDecoder(resp.Body).Decode(&entry)
	}
	return entry, resp.StatusCode, nil
}

// L1: Write to Leader, read from Leader after ACK → consistent
func TestLeaderConsistency_L1_ReadLeaderAfterWrite(t *testing.T) {
	key := fmt.Sprintf("l1_%d", time.Now().UnixNano())
	value := "consistent"

	written, status, err := setKey(leaderURL, key, value)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d", status)
	}

	got, status, err := getKey(leaderURL, key)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if got.Value != value {
		t.Errorf("value mismatch: got %q, want %q", got.Value, value)
	}
	if got.Version < written.Version {
		t.Errorf("version mismatch: got %d, want >= %d", got.Version, written.Version)
	}
}

// L2: Write to Leader, read from Follower after ACK → consistent
func TestLeaderConsistency_L2_ReadFollowerAfterWrite(t *testing.T) {
	key := fmt.Sprintf("l2_%d", time.Now().UnixNano())
	value := "replicated"

	written, status, err := setKey(leaderURL, key, value)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d", status)
	}

	// After leader ACK (W=5 strategy), all followers should have the data
	followers := []string{follower1URL, follower2URL, follower3URL, follower4URL}
	for _, f := range followers {
		got, status, err := getKey(f, key)
		if err != nil {
			t.Fatalf("get from %s failed: %v", f, err)
		}
		if status != http.StatusOK {
			t.Errorf("follower %s returned %d, expected 200", f, status)
			continue
		}
		if got.Value != value {
			t.Errorf("follower %s value: got %q, want %q", f, got.Value, value)
		}
		if got.Version < written.Version {
			t.Errorf("follower %s version: got %d, want >= %d", f, got.Version, written.Version)
		}
	}
}

// L3: Write to Leader, local_read from Followers within the update window → may be inconsistent
func TestLeaderConsistency_L3_LocalReadDuringReplication(t *testing.T) {
	key := fmt.Sprintf("l3_%d", time.Now().UnixNano())
	value := "during_replication"

	staleCount := 0
	attempts := 20

	for i := 0; i < attempts; i++ {
		iterKey := fmt.Sprintf("%s_%d", key, i)

		// Fire write to leader (don't wait for response)
		go setKey(leaderURL, iterKey, value)

		// Immediately read from followers via local_read
		time.Sleep(10 * time.Millisecond) // small delay to let the request reach the leader
		followers := []string{follower1URL, follower2URL, follower3URL, follower4URL}
		for _, f := range followers {
			_, status, err := localRead(f, iterKey)
			if err != nil {
				continue
			}
			if status == http.StatusNotFound {
				staleCount++
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Logf("Stale reads detected: %d out of %d follower reads (across %d writes)", staleCount, attempts*4, attempts)
	if staleCount == 0 {
		t.Log("WARNING: No stale reads detected. The inconsistency window may be too small or the test timing needs adjustment.")
	}
}

// Helper to check service is reachable
func TestLeaderClusterHealthCheck(t *testing.T) {
	urls := []string{leaderURL, follower1URL, follower2URL, follower3URL, follower4URL}
	for _, u := range urls {
		resp, err := http.Get(fmt.Sprintf("%s/get?key=healthcheck", u))
		if err != nil {
			t.Fatalf("node %s unreachable: %v", u, err)
		}
		resp.Body.Close()
		// 404 is fine — just checking the node is up
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("node %s returned unexpected status %d: %s", u, resp.StatusCode, body)
		}
	}
}
