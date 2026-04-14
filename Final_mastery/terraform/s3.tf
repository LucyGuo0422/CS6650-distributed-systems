# -----------------------------------------------------------------------------
# S3 Bucket for photo storage
# -----------------------------------------------------------------------------

resource "aws_s3_bucket" "photos" {
  bucket = "${var.project_name}-photos-${data.aws_caller_identity.current.account_id}"

  tags = { Name = "${var.project_name}-photos" }
}

resource "aws_s3_bucket_versioning" "photos" {
  bucket = aws_s3_bucket.photos.id
  versioning_configuration {
    status = "Disabled"
  }
}

resource "aws_s3_bucket_public_access_block" "photos" {
  bucket = aws_s3_bucket.photos.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "photos" {
  bucket = aws_s3_bucket.photos.id

  rule {
    id     = "cleanup-pending"
    status = "Enabled"

    filter {
      prefix = "photos/pending/"
    }

    expiration {
      days = 1
    }
  }
}
