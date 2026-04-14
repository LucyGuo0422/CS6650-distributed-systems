# -----------------------------------------------------------------------------
# Outputs
# -----------------------------------------------------------------------------

output "alb_dns_name" {
  description = "API endpoint (ALB DNS)"
  value       = aws_lb.api.dns_name
}

output "ecr_api_url" {
  description = "ECR repository URL for the API image"
  value       = aws_ecr_repository.api.repository_url
}

output "ecr_worker_url" {
  description = "ECR repository URL for the Worker image"
  value       = aws_ecr_repository.worker.repository_url
}

output "sqs_queue_url" {
  description = "SQS queue URL"
  value       = aws_sqs_queue.photos.url
}

output "s3_bucket" {
  description = "S3 bucket name"
  value       = aws_s3_bucket.photos.id
}

output "dynamodb_albums_table" {
  description = "DynamoDB albums table name"
  value       = aws_dynamodb_table.albums.name
}

output "dynamodb_photos_table" {
  description = "DynamoDB photos table name"
  value       = aws_dynamodb_table.photos.name
}
