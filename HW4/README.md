# Deploying Docker Image to AWS ECR

A step-by-step guide for building and pushing Docker images to Amazon Elastic Container Registry (ECR).

## Prerequisites

- AWS Account with Lab credentials
- Docker Desktop installed
- AWS CLI installed
- Go installed (for Go applications)

## Overview

This guide walks you through:
1. Configuring AWS credentials
2. Creating an ECR repository
3. Building and pushing a Docker image to ECR

---

## Step 1: Configure AWS CLI

### 1.1 Set up AWS credentials

```bash
aws configure
```

Enter when prompted:
- AWS Access Key ID
- AWS Secret Access Key
- Default region: `us-west-2` (match your lab region)
- Default output format: `json`

### 1.2 Set session token

```bash
aws configure set aws_session_token <YOUR-SESSION-TOKEN>
```

### 1.3 Verify configuration

```bash
aws configure list
```

Should display your access key, secret key, region, and session token.

### 1.4 Update region if needed

```bash
# Change region to match your AWS lab
aws configure set region us-west-2
```

---

## Step 2: Create ECR Repository

### Option A: Using AWS Console

1. Go to AWS Console (from Learner Lab)
2. Search for "ECR" in the top search bar
3. Click on "Elastic Container Registry"
4. Click "Create repository"
5. Configure:
   - Repository name: `hello-service`
   - Tag immutability: Immutable
   - Scan on push: Disabled
6. Click "Create repository"
7. Copy the Repository URI for later use

### Option B: Using AWS CLI

```bash
aws ecr create-repository \
  --repository-name hello-service \
  --region us-west-2
```

---

## Step 3: Prepare Your Application

### 3.1 Navigate to your project directory with Dockerfile

```bash
cd /path/to/your/project
```

### 3.2 Verify required files exist

```bash
ls -la
```

You should see:
- `Dockerfile` - Instructions for building the container
- `go.mod` - Go module dependencies
- `*.go` - Your Go application code files

### 3.3 Generate go.sum (if missing)

```bash
go mod tidy
```

This creates the `go.sum` file that tracks exact dependency versions.

---

## Step 4: Build Docker Builder (One-time setup)

### 4.1 Create the singlearch builder

```bash
docker buildx create --name singlearch --driver docker-container --use
```

### 4.2 Verify builder was created

```bash
docker buildx ls
```

---

## Step 5: Push Docker Image to ECR

### 5.1 Get your ECR repository URL

```bash
ECR_URL=$(aws ecr describe-repositories \
  --repository-names hello-service \
  --region us-west-2 \
  --query 'repositories[0].repositoryUri' \
  --output text)

echo "ECR URL: $ECR_URL"
```

**Example output:** `207955210549.dkr.ecr.us-west-2.amazonaws.com/hello-service`

### 5.2 Authenticate Docker to ECR

```bash
# Extract base URL
ECR_BASE=$(echo $ECR_URL | cut -d'/' -f1)

# Login to ECR (for Mac/Linux/Git Bash)
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin $ECR_BASE
```

**For Windows PowerShell:**
```powershell
$password = aws ecr get-login-password --region us-west-2
$password | docker login --username AWS --password-stdin $ECR_BASE
```

You should see: **"Login Succeeded"**

### 5.3 Build and push the image

```bash
docker buildx build \
  --builder singlearch \
  --platform linux/amd64 \
  --push \
  -t $ECR_URL:latest .
```

**What this does:**
- `docker buildx build` - Build the Docker image
- `--builder singlearch` - Use the custom builder
- `--platform linux/amd64` - Build for Intel/AMD architecture (AWS compatibility)
- `--push` - Automatically push to ECR after building
- `-t $ECR_URL:latest` - Tag the image with ECR URL and "latest" tag
- `.` - Use current directory (where Dockerfile is)

### 5.4 Verify the upload

```bash
aws ecr list-images \
  --repository-name hello-service \
  --region us-west-2 \
  --query 'imageIds[*].imageTag' \
  --output table
```

**Expected output:**
```
------------
|ListImages|
+----------+
|  latest  |
+----------+
```

---

## Quick Reference

### Key Commands Summary

```bash
# Configure AWS
aws configure
aws configure set aws_session_token <TOKEN>

# Create ECR repo
aws ecr create-repository --repository-name hello-service --region us-west-2

# Get ECR URL
ECR_URL=$(aws ecr describe-repositories --repository-names hello-service --region us-west-2 --query 'repositories[0].repositoryUri' --output text)

# Login to ECR
ECR_BASE=$(echo $ECR_URL | cut -d'/' -f1)
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin $ECR_BASE

# Build and push
docker buildx build --builder singlearch --platform linux/amd64 --push -t $ECR_URL:latest .

# Verify
aws ecr list-images --repository-name hello-service --region us-west-2 --query 'imageIds[*].imageTag' --output table
```