variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "sns_topic_arn" {
  description = "ARN of the Part II SNS topic (order-processing-events)"
  type        = string
}
