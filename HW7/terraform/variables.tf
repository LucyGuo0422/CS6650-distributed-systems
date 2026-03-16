variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "aws_account_id" {
  description = "AWS account ID"
  type        = string
}

variable "app_image" {
  description = "ECR image URI for the order service"
  type        = string
}

variable "num_workers" {
  description = "Number of SQS worker goroutines (Phase 5 scaling: 5, 20, 100)"
  type        = number
  default     = 1
}
