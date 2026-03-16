resource "aws_ecs_cluster" "main" {
  name = "order-service"
}

resource "aws_cloudwatch_log_group" "main" {
  name              = "/ecs/order-service"
  retention_in_days = 7
}

# ── IAM ──────────────────────────────────────────────────────────────────────
# AWS Academy Learner Lab does not allow iam:CreateRole.
# Use the pre-existing LabRole for both the execution role and task role.
# LabRole already has AmazonECSTaskExecutionRolePolicy + broad AWS access.
data "aws_iam_role" "lab" {
  name = "LabRole"
}

# ── Task Definition ───────────────────────────────────────────────────────────

resource "aws_ecs_task_definition" "main" {
  family                   = "order-service"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = data.aws_iam_role.lab.arn
  task_role_arn            = data.aws_iam_role.lab.arn

  container_definitions = jsonencode([{
    name  = "order-service"
    image = var.app_image
    portMappings = [{
      containerPort = 8080
      protocol      = "tcp"
    }]
    environment = [
      { name = "SNS_TOPIC_ARN", value = aws_sns_topic.orders.arn },
      { name = "SQS_QUEUE_URL", value = aws_sqs_queue.orders.url },
      { name = "AWS_REGION",    value = var.aws_region },
      # Phase 5: change num_workers variable to scale the processor goroutine pool
      { name = "NUM_WORKERS",   value = tostring(var.num_workers) }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.main.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

# ── ECS Service ───────────────────────────────────────────────────────────────

resource "aws_ecs_service" "main" {
  name            = "order-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.main.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = aws_subnet.private[*].id
    security_groups = [aws_security_group.ecs.id]
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.main.arn
    container_name   = "order-service"
    container_port   = 8080
  }

  depends_on = [aws_lb_listener.main]
}
