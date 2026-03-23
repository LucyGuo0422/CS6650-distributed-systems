resource "aws_dynamodb_table" "carts" {
  name         = "ShoppingCarts"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "N"
  }

  tags = { Name = "hw8-carts" }
}

# No IAM policy needed — LabRole already has DynamoDB permissions.
