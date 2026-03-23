variable "aws_region" {
  default = "us-west-2"
}
variable "db_password" {
  description = "RDS MySQL root password (unused in DynamoDB phase)"
  sensitive   = true
  default     = "Password123"
}
variable "app_image" {
  description = "ECR image URI, e.g. 123456789.dkr.ecr.us-east-1.amazonaws.com/hw8:latest"
}
variable "use_dynamo" {
  description = "Switch to DynamoDB mode"
  default     = false
}
