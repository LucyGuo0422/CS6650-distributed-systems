#!/usr/bin/env bash
set -euo pipefail

# Usage: ./deploy.sh [plan|apply|destroy|push-images]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
ACTION="${1:-plan}"

cd "$SCRIPT_DIR"

# Initialize terraform if needed
if [ ! -d ".terraform" ]; then
  echo "==> terraform init"
  terraform init
fi

case "$ACTION" in
  plan)
    terraform plan
    ;;

  apply)
    terraform apply
    ;;

  destroy)
    terraform destroy
    ;;

  push-images)
    # Get values from terraform output
    AWS_REGION=$(terraform output -raw aws_region 2>/dev/null || echo "us-west-2")
    ECR_API=$(terraform output -raw ecr_api_url)
    ECR_WORKER=$(terraform output -raw ecr_worker_url)
    ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

    echo "==> Logging in to ECR"
    aws ecr get-login-password --region "$AWS_REGION" | \
      docker login --username AWS --password-stdin "${ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

    echo "==> Building and pushing API image"
    docker build -t "$ECR_API:latest" -f "$PROJECT_DIR/Dockerfile.api" "$PROJECT_DIR"
    docker push "$ECR_API:latest"

    echo "==> Building and pushing Worker image"
    docker build -t "$ECR_WORKER:latest" -f "$PROJECT_DIR/Dockerfile.worker" "$PROJECT_DIR"
    docker push "$ECR_WORKER:latest"

    echo "==> Forcing new ECS deployment"
    CLUSTER=$(terraform output -raw ecs_cluster_name 2>/dev/null || echo "album-store")
    aws ecs update-service --cluster "$CLUSTER" --service album-store-api    --force-new-deployment --region "$AWS_REGION" >/dev/null
    aws ecs update-service --cluster "$CLUSTER" --service album-store-worker --force-new-deployment --region "$AWS_REGION" >/dev/null

    echo "==> Done. Services are redeploying."
    ;;

  *)
    echo "Usage: $0 [plan|apply|destroy|push-images]"
    exit 1
    ;;
esac
