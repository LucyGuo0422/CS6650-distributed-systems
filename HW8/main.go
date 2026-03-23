package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	_ "github.com/go-sql-driver/mysql"
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

// Cart structs
type CartItem struct {
	ItemID    int `json:"item_id"`
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type Cart struct {
	CartID     int        `json:"shopping_cart_id"`
	CustomerID int        `json:"customer_id"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	Items      []CartItem `json:"items"`
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

// Global DB connection (RDS/MySQL)
var db *sql.DB

// DynamoDB client
var dynamo *dynamodb.Client

const dynamoTable = "ShoppingCarts"

// DynamoCart embeds items directly (single-item design pattern)
type DynamoCart struct {
	CartID     int        `dynamodbav:"cart_id"`
	CustomerID int        `dynamodbav:"customer_id"`
	Status     string     `dynamodbav:"status"`
	CreatedAt  string     `dynamodbav:"created_at"`
	Items      []CartItem `dynamodbav:"items"`
}

func initDynamo() {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal("Failed to load AWS config:", err)
	}
	dynamo = dynamodb.NewFromConfig(cfg)
	log.Println("DynamoDB client ready.")
}

func initDB() {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		user, password, host, port, name)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Failed to open DB:", err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Retry loop — RDS takes a few seconds after ECS task starts
	for i := 0; i < 10; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("DB not ready, retrying in 3s... (%d/10)", i+1)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal("Cannot connect to DB:", err)
	}

	createTables()
	log.Println("Database ready.")
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS carts (
            cart_id     INT AUTO_INCREMENT PRIMARY KEY,
            customer_id INT NOT NULL,
            status      VARCHAR(20) NOT NULL DEFAULT 'active',
            created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            INDEX idx_customer_id (customer_id)
        )`,
		`CREATE TABLE IF NOT EXISTS cart_items (
            item_id    INT AUTO_INCREMENT PRIMARY KEY,
            cart_id    INT NOT NULL,
            product_id INT NOT NULL,
            quantity   INT NOT NULL DEFAULT 1,
            FOREIGN KEY (cart_id) REFERENCES carts(cart_id) ON DELETE CASCADE,
            INDEX idx_cart_id (cart_id),
            UNIQUE KEY uq_cart_product (cart_id, product_id)
        )`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			log.Fatal("Failed to create table:", err)
		}
	}
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
	maxCheck := 100 // Bounded iteration: ALWAYS check exactly 100 products
	maxReturn := 20 // Return at most 20 results

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

// createCartHandler handles POST /shopping-carts
func createCartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		CustomerID int `json:"customer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CustomerID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT","message":"customer_id required"}`, http.StatusBadRequest)
		return
	}

	result, err := db.Exec("INSERT INTO carts (customer_id) VALUES (?)", body.CustomerID)
	if err != nil {
		log.Println("createCart error:", err)
		http.Error(w, `{"error":"SERVER_ERROR","message":"failed to create cart"}`, http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"shopping_cart_id": int(id)})
}

// getCartHandler handles GET /shopping-carts/{id}
func getCartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse ID from path: /shopping-carts/42
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/shopping-carts/"), "/")
	cartID := 0
	fmt.Sscanf(parts[0], "%d", &cartID)
	if cartID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT","message":"invalid cart id"}`, http.StatusBadRequest)
		return
	}

	// Fetch cart row
	var cart Cart
	err := db.QueryRow(
		"SELECT cart_id, customer_id, status, created_at FROM carts WHERE cart_id = ?", cartID,
	).Scan(&cart.CartID, &cart.CustomerID, &cart.Status, &cart.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"NOT_FOUND","message":"cart not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"SERVER_ERROR","message":"db error"}`, http.StatusInternalServerError)
		return
	}

	// Fetch items
	rows, err := db.Query(
		"SELECT item_id, product_id, quantity FROM cart_items WHERE cart_id = ?", cartID,
	)
	if err != nil {
		http.Error(w, `{"error":"SERVER_ERROR","message":"db error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	cart.Items = []CartItem{}
	for rows.Next() {
		var item CartItem
		rows.Scan(&item.ItemID, &item.ProductID, &item.Quantity)
		cart.Items = append(cart.Items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cart)
}

// addItemsHandler handles POST /shopping-carts/{id}/items
func addItemsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse cart ID from path: /shopping-carts/42/items
	parts := strings.Split(r.URL.Path, "/")
	cartID := 0
	for i, p := range parts {
		if p == "shopping-carts" && i+1 < len(parts) {
			fmt.Sscanf(parts[i+1], "%d", &cartID)
		}
	}
	if cartID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT","message":"invalid cart id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProductID < 1 || body.Quantity < 1 {
		http.Error(w, `{"error":"INVALID_INPUT","message":"product_id and quantity required"}`, http.StatusBadRequest)
		return
	}

	// Verify cart exists
	var exists int
	db.QueryRow("SELECT 1 FROM carts WHERE cart_id = ?", cartID).Scan(&exists)
	if exists == 0 {
		http.Error(w, `{"error":"NOT_FOUND","message":"cart not found"}`, http.StatusNotFound)
		return
	}

	// INSERT or UPDATE quantity if product already in cart
	_, err := db.Exec(`
		INSERT INTO cart_items (cart_id, product_id, quantity)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)
	`, cartID, body.ProductID, body.Quantity)
	if err != nil {
		log.Println("addItems error:", err)
		http.Error(w, `{"error":"SERVER_ERROR","message":"failed to add item"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// createCartDynamoHandler handles POST /shopping-carts (DynamoDB)
func createCartDynamoHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CustomerID int `json:"customer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.CustomerID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT"}`, http.StatusBadRequest)
		return
	}

	// Timestamp-based ID for uniqueness
	cartID := int(time.Now().UnixNano() % 1_000_000_000)

	cart := DynamoCart{
		CartID:     cartID,
		CustomerID: body.CustomerID,
		Status:     "active",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		Items:      []CartItem{},
	}

	item, _ := attributevalue.MarshalMap(cart)
	_, err := dynamo.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(dynamoTable),
		Item:      item,
	})
	if err != nil {
		log.Println("createCartDynamo error:", err)
		http.Error(w, `{"error":"SERVER_ERROR"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"shopping_cart_id": cartID})
}

// getCartDynamoHandler handles GET /shopping-carts/{id} (DynamoDB)
func getCartDynamoHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/shopping-carts/"), "/")
	cartID := 0
	fmt.Sscanf(parts[0], "%d", &cartID)
	if cartID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT"}`, http.StatusBadRequest)
		return
	}

	out, err := dynamo.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(dynamoTable),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", cartID)},
		},
	})
	if err != nil || out.Item == nil {
		http.Error(w, `{"error":"NOT_FOUND"}`, http.StatusNotFound)
		return
	}

	var cart DynamoCart
	attributevalue.UnmarshalMap(out.Item, &cart)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cart)
}

// addItemsDynamoHandler handles POST /shopping-carts/{id}/items (DynamoDB)
func addItemsDynamoHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	cartID := 0
	for i, p := range parts {
		if p == "shopping-carts" && i+1 < len(parts) {
			fmt.Sscanf(parts[i+1], "%d", &cartID)
		}
	}
	if cartID < 1 {
		http.Error(w, `{"error":"INVALID_INPUT"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProductID < 1 || body.Quantity < 1 {
		http.Error(w, `{"error":"INVALID_INPUT"}`, http.StatusBadRequest)
		return
	}

	newItem, _ := attributevalue.MarshalMap(CartItem{
		ProductID: body.ProductID,
		Quantity:  body.Quantity,
	})

	_, err := dynamo.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(dynamoTable),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", cartID)},
		},
		UpdateExpression:         aws.String("SET #items = list_append(if_not_exists(#items, :empty), :newItem)"),
		ExpressionAttributeNames: map[string]string{"#items": "items"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":newItem": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberM{Value: newItem},
			}},
			":empty": &types.AttributeValueMemberL{Value: []types.AttributeValue{}},
		},
	})
	if err != nil {
		log.Println("addItemsDynamo error:", err)
		http.Error(w, `{"error":"SERVER_ERROR"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	generateProducts()

	useDynamo := os.Getenv("USE_DYNAMO") == "true"

	if useDynamo {
		initDynamo()
		http.HandleFunc("/shopping-carts", createCartDynamoHandler)
		http.HandleFunc("/shopping-carts/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/items") {
				addItemsDynamoHandler(w, r)
			} else {
				getCartDynamoHandler(w, r)
			}
		})
	} else {
		initDB()
		http.HandleFunc("/shopping-carts", createCartHandler)
		http.HandleFunc("/shopping-carts/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/items") {
				addItemsHandler(w, r)
			} else {
				getCartHandler(w, r)
			}
		})
	}

	http.HandleFunc("/products/search", searchHandler)
	http.HandleFunc("/health", healthHandler)

	port := "8080"
	log.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
