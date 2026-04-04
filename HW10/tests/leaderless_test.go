package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

var leaderlessNodes = []string{
	"http://localhost:9081",
	"http://localhost:9082",
	"http://localhost:9083",
	"http://localhost:9084",
	"http://localhost:9085",
}

// LL1: Write to node1, get from node2 within sync window → inconsistent
func TestLeaderlessConsistency_LL1_ReadDuringSyncWindow(t *testing.T) {
	staleCount := 0
	attempts := 20

	for i := 0; i < attempts; i++ {
		key := fmt.Sprintf("ll1_%d_%d", time.Now().UnixNano(), i)
		coordinator := leaderlessNodes[0]

		// Fire write to coordinator (don't wait for response)
		go setKey(coordinator, key, "value")

		// Immediately try reading from other nodes
		time.Sleep(10 * time.Millisecond)
		for _, node := range leaderlessNodes[1:] {
			_, status, err := getKey(node, key)
			if err != nil {
				continue
			}
			if status == http.StatusNotFound {
				staleCount++
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Logf("Stale reads detected: %d out of %d node reads (across %d writes)", staleCount, attempts*4, attempts)
	if staleCount == 0 {
		t.Log("WARNING: No stale reads detected. The inconsistency window may be too small.")
	}
}

// LL2: Write to node1, read from node1 after ACK → consistent
func TestLeaderlessConsistency_LL2_ReadCoordinatorAfterWrite(t *testing.T) {
	key := fmt.Sprintf("ll2_%d", time.Now().UnixNano())
	value := "coordinator_consistent"
	coordinator := leaderlessNodes[0]

	written, status, err := setKey(coordinator, key, value)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d", status)
	}

	got, status, err := getKey(coordinator, key)
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

// LL3: Write to node1, read from node2 after ACK → consistent
func TestLeaderlessConsistency_LL3_ReadOtherNodeAfterWrite(t *testing.T) {
	key := fmt.Sprintf("ll3_%d", time.Now().UnixNano())
	value := "all_consistent"
	coordinator := leaderlessNodes[0]

	written, status, err := setKey(coordinator, key, value)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected 201, got %d", status)
	}

	// After coordinator ACK (W=N), all nodes should have the data
	for _, node := range leaderlessNodes[1:] {
		got, status, err := getKey(node, key)
		if err != nil {
			t.Fatalf("get from %s failed: %v", node, err)
		}
		if status != http.StatusOK {
			t.Errorf("node %s returned %d, expected 200", node, status)
			continue
		}
		if got.Value != value {
			t.Errorf("node %s value: got %q, want %q", node, got.Value, value)
		}
		if got.Version < written.Version {
			t.Errorf("node %s version: got %d, want >= %d", node, got.Version, written.Version)
		}
	}
}

// Health check for leaderless cluster
func TestLeaderlessClusterHealthCheck(t *testing.T) {
	for _, u := range leaderlessNodes {
		resp, err := http.Get(fmt.Sprintf("%s/get?key=healthcheck", u))
		if err != nil {
			t.Fatalf("node %s unreachable: %v", u, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("node %s returned unexpected status %d", u, resp.StatusCode)
		}
	}
}
