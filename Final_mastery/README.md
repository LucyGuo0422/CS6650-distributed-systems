# Album Store - ChaosArena v1-album-store Implementation

High-performance REST API for album and photo management with async photo processing.

## Architecture

```
┌─────────┐      ┌───────────────┐      ┌──────────┐
│   ALB   │─────▶│  API Service  │─────▶│ DynamoDB │
└─────────┘      │  (ECS Fargate)│      │  Tables  │
                 └───────┬───────┘      └──────────┘
                         │
                         ├─────────────▶┌──────────┐
                         │               │    S3    │
                         │               │  Bucket  │
                         │               └──────────┘
                         │
                         ├─────────────▶┌──────────┐
                         │               │   SQS    │
                         │               │  Queue   │
                         │               └─────┬────┘
                         │                     │
                         │                     ▼
                         │              ┌──────────────┐
                         └──────────────│    Worker    │
                                        │ (ECS Fargate)│
                                        └──────────────┘
```

## Tech Stack

- **Language**: Go 1.25
- **Web Framework**: Gin
- **Database**: AWS DynamoDB (on-demand)
- **File Storage**: AWS S3 (public read)
- **Queue**: AWS SQS
- **Deployment**: AWS ECS Fargate + ALB
- **Container**: Docker multi-stage build

## Project Structure

```
album-store/
├── cmd/
│   ├── api/main.go          # API server entry point
│   └── worker/main.go       # Async worker entry point
├── internal/
│   ├── handler/
│   │   ├── album.go         # Album HTTP handlers
│   │   └── photo.go         # Photo HTTP handlers
│   ├── store/
│   │   ├── dynamodb.go      # DynamoDB operations
│   │   ├── s3.go            # S3 operations
│   │   └── memory.go        # In-memory (for testing)
│   └── queue/
│       └── sqs.go           # SQS operations
├── terraform/
│   ├── main.tf                 # Provider & backend config
│   ├── variables.tf            # Tunables (region, counts, CPU/mem)
│   ├── vpc.tf                  # VPC, subnets, IGW, NAT
│   ├── dynamodb.tf             # Albums + Photos tables
│   ├── s3.tf                   # Photo bucket + lifecycle
│   ├── sqs.tf                  # Processing queue + DLQ
│   ├── ecr.tf                  # Container registries
│   ├── iam.tf                  # Execution & task roles
│   ├── alb.tf                  # Application Load Balancer
│   ├── ecs.tf                  # Fargate cluster & services
│   ├── outputs.tf              # Resource identifiers
│   └── deploy.sh               # Build, push, deploy helper
├── Dockerfile.api          # API Docker build
├── Dockerfile.worker       # Worker Docker build
├── docker-compose.yml      # Local development
└── go.mod                  # Go dependencies
```

## API Endpoints

### Health Check
- `GET /health` - Returns `{"status": "ok"}`

### Albums
- `PUT /albums/:album_id` - Create/update album
- `GET /albums/:album_id` - Get album by ID
- `GET /albums` - List all albums

### Photos
- `POST /albums/:album_id/photos` - Upload photo (returns 202 immediately)
- `GET /albums/:album_id/photos/:photo_id` - Get photo details
- `DELETE /albums/:album_id/photos/:photo_id` - Delete photo

## Key Design Decisions

### DynamoDB Schema
- **Flat schema** with `photo_id` as partition key (no GSI needed)
- **Seq counter** stored as `SEQ#<album_id>` in photos table
- Atomic increment using `UpdateItem + ADD`

### Photo Upload Flow
1. Generate UUID for photo_id
2. Atomically increment seq counter
3. Write DynamoDB record (status=processing, URL pre-computed)
4. Read file into memory buffer
5. Launch goroutine to upload file to staging location (photos/pending/{photo_id})
6. Return 202 immediately
7. **Goroutine** uploads file to S3 staging, then sends photo_id to SQS
8. **Worker** polls SQS, moves staging→final, updates status to "completed"

### Delete Order
1. GetItem from DynamoDB (fetch record)
2. DeleteObject from S3 **(before DynamoDB)**
3. DeleteItem from DynamoDB

Deleting S3 first prevents orphaned files.

### Performance Optimizations
- AWS SDK clients initialized **once** at startup
- HTTP transport: `MaxIdleConnsPerHost: 200`
- Retry configuration: max 3 attempts with exponential backoff
- HTTP server timeouts: Read 30s, Write 30s, Idle 120s
- Graceful shutdown on SIGTERM
- Worker concurrency: configurable (default 50)
- SQS long-polling: 20 seconds
- SQS dead-letter queue after 3 failed receives

## Local Development

### Using Docker Compose

```bash
# Set environment variables
export S3_BUCKET=your-bucket-name
export SQS_QUEUE_URL=https://sqs.us-west-2.amazonaws.com/ACCOUNT/QUEUE
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

# Start services
docker-compose up --build

# API available at http://localhost:8080
```

### Running Locally (without Docker)

```bash
# Set environment variables
export AWS_REGION=us-west-2
export DYNAMODB_ALBUMS_TABLE=albums
export DYNAMODB_PHOTOS_TABLE=photos
export S3_BUCKET=your-bucket-name
export SQS_QUEUE_URL=https://sqs...

# Run API
go run cmd/api/main.go

# Run Worker (in another terminal)
go run cmd/worker/main.go
```

## Deployment to AWS (Terraform)

All infrastructure is defined in `terraform/`. Requires Terraform >= 1.5 and valid AWS credentials.

```bash
cd terraform

# 1. Initialize and preview
terraform init
terraform plan

# 2. Create all infrastructure
terraform apply

# 3. Build Docker images, push to ECR, and redeploy ECS
./deploy.sh push-images
```

The ALB endpoint is printed as an output:
```bash
terraform output alb_dns_name
```

To tear down everything:
```bash
terraform destroy
```

### Configurable Variables

| Variable | Default | Description |
|---|---|---|
| `aws_region` | `us-west-2` | AWS region |
| `api_desired_count` | `2` | Number of API tasks |
| `worker_desired_count` | `4` | Number of Worker tasks |
| `worker_concurrency` | `50` | Goroutines per worker |
| `api_cpu` / `api_memory` | `512` / `1024` | API task sizing |
| `worker_cpu` / `worker_memory` | `512` / `1024` | Worker task sizing |

Override with `-var`:
```bash
terraform apply -var="api_desired_count=3" -var="worker_desired_count=6"
```

## Testing

```bash
# Health check
curl http://localhost:8080/health

# Create album
curl -X PUT http://localhost:8080/albums/album1 \
  -H "Content-Type: application/json" \
  -d '{"title":"My Album","description":"Test","owner":"user1"}'

# Get album
curl http://localhost:8080/albums/album1

# Upload photo
echo "test image data" > test.jpg
curl -X POST http://localhost:8080/albums/album1/photos \
  -F "photo=@test.jpg"

# Get photo (check status)
curl http://localhost:8080/albums/album1/photos/<photo_id>

# Delete photo
curl -X DELETE http://localhost:8080/albums/album1/photos/<photo_id>
```

## Monitoring

### CloudWatch Logs
- API: `/ecs/album-store-api`
- Worker: `/ecs/album-store-worker`

```bash
aws logs tail /ecs/album-store-api --follow --region us-west-2
```

### ECS Service Status
```bash
aws ecs describe-services \
  --cluster album-store \
  --services album-store-api album-store-worker \
  --region us-west-2
```

## Environment Variables

### API Server
- `PORT` - Listen port (default: 8080)
- `AWS_REGION` - AWS region
- `DYNAMODB_ALBUMS_TABLE` - Albums table name
- `DYNAMODB_PHOTOS_TABLE` - Photos table name
- `S3_BUCKET` - S3 bucket name
- `SQS_QUEUE_URL` - SQS queue URL

### Worker
- `AWS_REGION` - AWS region
- `DYNAMODB_PHOTOS_TABLE` - Photos table name
- `SQS_QUEUE_URL` - SQS queue URL
- `WORKER_CONCURRENCY` - Goroutine concurrency (default: 50)

## ChaosArena Compliance

- ✅ Exact health check response: `{"status": "ok"}`
- ✅ PUT idempotency (uses path album_id as key)
- ✅ GET /albums returns ALL albums with full pagination
- ✅ POST returns 202 with status=processing
- ✅ Seq is atomic per album
- ✅ S3 URLs are publicly accessible
- ✅ DELETE order: S3 before DynamoDB
- ✅ After DELETE: GET returns 404, S3 URL returns non-200
- ✅ No bucket versioning
- ✅ Worker is only code path that writes status=completed
- ✅ ECS desired count ≥ 2 for warm baseline

## License

MIT
