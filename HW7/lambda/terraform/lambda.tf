terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# Use the pre-existing LabRole (AWS Academy Learner Labs block iam:CreateRole)
data "aws_iam_role" "lab_role" {
  name = "LabRole"
}

# Lambda function
resource "aws_lambda_function" "order_processor" {
  function_name = "order-processor-lambda"
  role          = data.aws_iam_role.lab_role.arn
  runtime       = "provided.al2"
  handler       = "bootstrap"       # must match binary name
  filename      = "../function.zip" # built by make build
  memory_size   = 512
  timeout       = 30                # must exceed 3s processing time

  environment {
    variables = {
      ENV = "production"
    }
  }
}

# Allow SNS to invoke the Lambda
resource "aws_lambda_permission" "allow_sns" {
  statement_id  = "AllowSNSInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.order_processor.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = var.sns_topic_arn
}

# Subscribe Lambda to the existing SNS topic
resource "aws_sns_topic_subscription" "lambda_sub" {
  topic_arn = var.sns_topic_arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.order_processor.arn
}

output "lambda_arn" {
  value = aws_lambda_function.order_processor.arn
}
