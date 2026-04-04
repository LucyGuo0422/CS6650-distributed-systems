package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type kvEntry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type keyState struct {
	mu            sync.Mutex
	latestVersion int64
	lastWriteTime time.Time
}

type readRecord struct {
	Timestamp       time.Time
	Key             string
	LatencyMs       float64
	IsStale         bool
	ReturnedVersion int64
	ExpectedVersion int64
	Node            string
}

type writeRecord struct {
	Timestamp      time.Time
	Key            string
	LatencyMs      float64
	VersionWritten int64
	Node           string
}

type intervalRecord struct {
	Key                 string
	TimeSinceLastWriteMs float64
}

func main() {
	mode := flag.String("mode", "leader", "leader or leaderless")
	leaderURL := flag.String("leader", "", "Leader URL (leader mode)")
	followersStr := flag.String("followers", "", "Comma-separated follower URLs (leader mode)")
	nodesStr := flag.String("nodes", "", "Comma-separated node URLs (leaderless mode)")
	duration := flag.Int("duration", 60, "Test duration in seconds")
	writeRatio := flag.Float64("write-ratio", 0.1, "Fraction of requests that are writes")
	numKeys := flag.Int("keys", 30, "Number of distinct keys")
	concurrency := flag.Int("concurrency", 10, "Number of concurrent goroutines")
	outDir := flag.String("out", "./results", "Output directory for CSV files")
	flag.Parse()

	// Parse node URLs
	var writeTargets []string // nodes that accept writes
	var readTargets []string  // nodes that accept reads

	switch *mode {
	case "leader":
		if *leaderURL == "" {
			log.Fatal("--leader is required in leader mode")
		}
		writeTargets = []string{*leaderURL}
		readTargets = []string{*leaderURL}
		if *followersStr != "" {
			for _, f := range strings.Split(*followersStr, ",") {
				f = strings.TrimSpace(f)
				if f != "" {
					readTargets = append(readTargets, f)
				}
			}
		}
	case "leaderless":
		if *nodesStr == "" {
			log.Fatal("--nodes is required in leaderless mode")
		}
		for _, n := range strings.Split(*nodesStr, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				writeTargets = append(writeTargets, n)
				readTargets = append(readTargets, n)
			}
		}
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}

	// Build key pool
	keys := make([]string, *numKeys)
	for i := 0; i < *numKeys; i++ {
		keys[i] = fmt.Sprintf("key_%d", i)
	}

	// Key state tracking
	states := make(map[string]*keyState)
	for _, k := range keys {
		states[k] = &keyState{}
	}

	// Result collectors
	var readsMu sync.Mutex
	var reads []readRecord
	var writesMu sync.Mutex
	var writes []writeRecord
	var intervalsMu sync.Mutex
	var intervals []intervalRecord

	log.Printf("Starting load test: mode=%s, duration=%ds, write-ratio=%.2f, keys=%d, concurrency=%d",
		*mode, *duration, *writeRatio, *numKeys, *concurrency)
	log.Printf("Write targets: %v", writeTargets)
	log.Printf("Read targets: %v", readTargets)

	deadline := time.Now().Add(time.Duration(*duration) * time.Second)
	var wg sync.WaitGroup

	for g := 0; g < *concurrency; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(rand.Intn(10000))))
			client := &http.Client{Timeout: 10 * time.Second}

			for time.Now().Before(deadline) {
				key := keys[rng.Intn(len(keys))]
				isWrite := rng.Float64() < *writeRatio

				if isWrite {
					target := writeTargets[rng.Intn(len(writeTargets))]
					value := fmt.Sprintf("v_%d", time.Now().UnixNano())

					start := time.Now()
					entry, err := doSet(client, target, key, value)
					latency := time.Since(start).Seconds() * 1000

					if err != nil {
						continue
					}

					st := states[key]
					st.mu.Lock()
					st.latestVersion = entry.Version
					st.lastWriteTime = time.Now()
					st.mu.Unlock()

					writesMu.Lock()
					writes = append(writes, writeRecord{
						Timestamp:      time.Now(),
						Key:            key,
						LatencyMs:      latency,
						VersionWritten: entry.Version,
						Node:           target,
					})
					writesMu.Unlock()
				} else {
					target := readTargets[rng.Intn(len(readTargets))]

					start := time.Now()
					entry, status, err := doGet(client, target, key)
					latency := time.Since(start).Seconds() * 1000

					if err != nil {
						continue
					}

					st := states[key]
					st.mu.Lock()
					expectedVersion := st.latestVersion
					lastWrite := st.lastWriteTime
					st.mu.Unlock()

					isStale := false
					returnedVersion := int64(0)
					if status == http.StatusOK {
						returnedVersion = entry.Version
						if returnedVersion < expectedVersion {
							isStale = true
						}
					} else if status == http.StatusNotFound && expectedVersion > 0 {
						isStale = true
					}

					readsMu.Lock()
					reads = append(reads, readRecord{
						Timestamp:       time.Now(),
						Key:             key,
						LatencyMs:       latency,
						IsStale:         isStale,
						ReturnedVersion: returnedVersion,
						ExpectedVersion: expectedVersion,
						Node:            target,
					})
					readsMu.Unlock()

					if !lastWrite.IsZero() {
						intervalsMu.Lock()
						intervals = append(intervals, intervalRecord{
							Key:                 key,
							TimeSinceLastWriteMs: time.Since(lastWrite).Seconds() * 1000,
						})
						intervalsMu.Unlock()
					}
				}
			}
		}()
	}

	wg.Wait()

	// Count stale reads
	staleCount := 0
	for _, r := range reads {
		if r.IsStale {
			staleCount++
		}
	}
	log.Printf("Done. Writes: %d, Reads: %d, Stale reads: %d", len(writes), len(reads), staleCount)

	// Write CSVs
	os.MkdirAll(*outDir, 0755)
	writeReadsCSV(filepath.Join(*outDir, "reads.csv"), reads)
	writeWritesCSV(filepath.Join(*outDir, "writes.csv"), writes)
	writeIntervalsCSV(filepath.Join(*outDir, "intervals.csv"), intervals)

	log.Printf("Results written to %s", *outDir)
}

func doSet(client *http.Client, baseURL, key, value string) (kvEntry, error) {
	url := fmt.Sprintf("%s/set?key=%s&value=%s", baseURL, key, value)
	resp, err := client.Post(url, "", nil)
	if err != nil {
		return kvEntry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return kvEntry{}, fmt.Errorf("set returned %d", resp.StatusCode)
	}
	var entry kvEntry
	json.NewDecoder(resp.Body).Decode(&entry)
	return entry, nil
}

func doGet(client *http.Client, baseURL, key string) (kvEntry, int, error) {
	url := fmt.Sprintf("%s/get?key=%s", baseURL, key)
	resp, err := client.Get(url)
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

func writeReadsCSV(path string, records []readRecord) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"timestamp", "key", "latency_ms", "is_stale", "returned_version", "expected_version", "node"})
	for _, r := range records {
		w.Write([]string{
			r.Timestamp.Format(time.RFC3339Nano),
			r.Key,
			fmt.Sprintf("%.2f", r.LatencyMs),
			fmt.Sprintf("%t", r.IsStale),
			fmt.Sprintf("%d", r.ReturnedVersion),
			fmt.Sprintf("%d", r.ExpectedVersion),
			r.Node,
		})
	}
	w.Flush()
}

func writeWritesCSV(path string, records []writeRecord) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"timestamp", "key", "latency_ms", "version_written", "node"})
	for _, r := range records {
		w.Write([]string{
			r.Timestamp.Format(time.RFC3339Nano),
			r.Key,
			fmt.Sprintf("%.2f", r.LatencyMs),
			fmt.Sprintf("%d", r.VersionWritten),
			r.Node,
		})
	}
	w.Flush()
}

func writeIntervalsCSV(path string, records []intervalRecord) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"key", "time_since_last_write_ms"})
	for _, r := range records {
		w.Write([]string{
			r.Key,
			fmt.Sprintf("%.2f", r.TimeSinceLastWriteMs),
		})
	}
	w.Flush()
}
