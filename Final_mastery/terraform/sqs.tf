# -----------------------------------------------------------------------------
# SQS Queue for photo processing
# -----------------------------------------------------------------------------

resource "aws_sqs_queue" "photos" {
  name                       = "${var.project_name}-photo-processing"
  visibility_timeout_seconds = 60
  message_retention_seconds  = 86400   # 1 day
  receive_wait_time_seconds  = 20      # long-polling (matches worker code)

  tags = { Name = "${var.project_name}-photo-processing" }
}

resource "aws_sqs_queue" "photos_dlq" {
  name                      = "${var.project_name}-photo-processing-dlq"
  message_retention_seconds = 604800 # 7 days

  tags = { Name = "${var.project_name}-photo-processing-dlq" }
}

resource "aws_sqs_queue_redrive_policy" "photos" {
  queue_url = aws_sqs_queue.photos.id

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.photos_dlq.arn
    maxReceiveCount     = 3
  })
}
