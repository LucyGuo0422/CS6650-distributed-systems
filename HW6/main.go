package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Product represents a single product in the catalog
type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

// SearchResponse is what we return to the client
type SearchResponse struct {
	Products   []Product `json:"products"`
	TotalFound int       `json:"total_found"`
	SearchTime string    `json:"search_time"`
}

// Global thread-safe store
var productStore sync.Map
var totalProducts = 100_000

// Sample values for generation
var brands = []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Omega", "Nova"}
var categories = []string{"Electronics", "Books", "Home", "Sports", "Clothing", "Toys", "Garden", "Automotive"}
var descriptions = []string{
	"High quality product for everyday use",
	"Premium grade, built to last",
	"Affordable and reliable choice",
	"Top-rated by customers worldwide",
	"Innovative design meets functionality",
}

func generateProducts() {
	log.Println("Generating 100,000 products...")
	for i := 1; i <= totalProducts; i++ {
		brand := brands[i%len(brands)]
		category := categories[i%len(categories)]
		p := Product{
			ID:          i,
			Name:        fmt.Sprintf("Product %s %d", brand, i),
			Category:    category,
			Description: descriptions[i%len(descriptions)],
			Brand:       brand,
		}
		productStore.Store(i, p)
	}
	log.Printf("Done! %d products loaded into memory.\n", totalProducts)
}

// searchHandler handles GET /products/search?q=<query>
func searchHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	query := strings.ToLower(r.URL.Query().Get("q"))

	var results []Product
	var totalFound int
	checked := 0
	maxCheck := 100  // Bounded iteration: ALWAYS check exactly 100 products
	maxReturn := 20  // Return at most 20 results

	// Iterate through products, stopping after 100 checks
	for i := 1; i <= totalProducts; i++ {
		if checked >= maxCheck {
			break
		}

		val, ok := productStore.Load(i)
		if !ok {
			continue
		}

		checked++ // Increment for EVERY product we examine

		p := val.(Product)
		nameLower := strings.ToLower(p.Name)
		catLower := strings.ToLower(p.Category)

		if strings.Contains(nameLower, query) || strings.Contains(catLower, query) {
			totalFound++
			if len(results) < maxReturn {
				results = append(results, p)
			}
		}
	}

	elapsed := time.Since(start)
	resp := SearchResponse{
		Products:   results,
		TotalFound: totalFound,
		SearchTime: elapsed.String(),
	}

	if results == nil {
		resp.Products = []Product{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// healthHandler for ALB / ECS health checks
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	generateProducts()

	http.HandleFunc("/products/search", searchHandler)
	http.HandleFunc("/health", healthHandler)

	port := "8080"
	log.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}