# Product API

A simple Go-based REST API for managing products, implementing the Product endpoints from the e-commerce OpenAPI specification.

## Project Structure

```
product-api/
├── src/
│   ├── main.go            # Product API server
│   ├── go.mod
│   └── Dockerfile         # Container build file
├── terraform/
│   ├── main.tf            # Root module (wires network, ecr, logging, ecs)
│   ├── variables.tf
│   ├── provider.tf        # AWS & Docker provider config
│   ├── outputs.tf
│   └── modules/
│       ├── ecr/           # ECR repository
│       ├── ecs/           # ECS cluster, task definition, service
│       ├── logging/       # CloudWatch log group
│       └── network/       # VPC, subnets, security group
├── locust-test/
│   ├── locustfile.py      # Load test (FastHttpUser)
│   └── stress_test.py     # Stress test (zero wait time)
└── README.md
```

## Endpoints

| Method | Path                                        | Description                 | Success Code |
| ------ | ------------------------------------------- | --------------------------- | ------------ |
| GET    | `/products/{productId}`                     | Get product by ID           | 200          |
| POST   | `/products/{productId}/details`             | Add/update product          | 204          |


## How to Deploy

### Option 1: Run Locally

```bash
cd src
go run main.go
# Server starts at http://localhost:8080
```

### Option 2: Run with Docker

```bash
cd src
docker build -t product-api .
docker run -p 8080:8080 product-api
```

### Option 3: Deploy to AWS with Terraform

**Prerequisites**: AWS CLI configured, Terraform installed, Docker installed.

```bash
cd terraform
terraform init
terraform apply -auto-approve
```

Get the public IP from:
```bash
terraform output
```

To tear down:
```bash
terraform destroy -auto-approve
```

## API Examples & Response Codes

### 204 — Add product (success)

```bash
curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": 1,
    "sku": "ABC-123-XYZ",
    "manufacturer": "Acme Corporation",
    "category_id": 456,
    "weight": 1250,
    "some_other_id": 789
  }'
# Response: 204 No Content
```

### 200 — Get product (success)

```bash
curl http://localhost:8080/products/1
# Response: 200 OK
# {
#   "product_id": 1,
#   "sku": "ABC-123-XYZ",
#   "manufacturer": "Acme Corporation",
#   "category_id": 456,
#   "weight": 1250,
#   "some_other_id": 789
# }
```

### 404 — Product not found

```bash
curl http://localhost:8080/products/999
# Response: 404 Not Found
# {"error":"NOT_FOUND","message":"product not found"}
```

### 400 — Invalid JSON body

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d 'not valid json'
# Response: 400 Bad Request
# {"error":"INVALID_INPUT","message":"invalid JSON body"}
```

### 400 — Validation error (empty SKU)

```bash
curl -X POST http://localhost:8080/products/2/details \
  -H "Content-Type: application/json" \
  -d '{"product_id":2,"sku":"","manufacturer":"Test","category_id":1,"weight":0,"some_other_id":1}'
# Response: 400 Bad Request
# {"error":"INVALID_INPUT","message":"sku must be 1-100 characters"}
```

### 400 — Product ID mismatch (URL vs body)

```bash
curl -X POST http://localhost:8080/products/1/details \
  -H "Content-Type: application/json" \
  -d '{"product_id":99,"sku":"ABC","manufacturer":"Test","category_id":1,"weight":0,"some_other_id":1}'
# Response: 400 Bad Request
# {"error":"INVALID_INPUT","message":"product_id in body must match URL"}
```

### 405 — Method not allowed

```bash
curl -X DELETE http://localhost:8080/products/1
# Response: 405 Method Not Allowed
# {"error":"METHOD_NOT_ALLOWED","message":"method not allowed"}
```

## Load Testing

See the `locust-test/` directory for test scripts.

```bash
cd locust-test

# Load test (80% reads, 20% writes, 1-3s wait between requests)
locust -f locustfile.py --host=http://<SERVER_IP>:8080

# Stress test (zero wait time, max throughput)
locust -f stress_test.py --host=http://<SERVER_IP>:8080
```

Open http://localhost:8089 to configure and run tests.