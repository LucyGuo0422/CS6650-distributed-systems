resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags = { Name = "order-service-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "order-service-igw" }
}

# Public subnets — ALB lives here
resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.main.id
  cidr_block              = ["10.0.1.0/24", "10.0.2.0/24"][count.index]
  availability_zone       = ["us-west-2a", "us-west-2b"][count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "order-service-public-${count.index + 1}" }
}

# Private subnets — ECS tasks live here
resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = ["10.0.10.0/24", "10.0.11.0/24"][count.index]
  availability_zone = ["us-west-2a", "us-west-2b"][count.index]
  tags              = { Name = "order-service-private-${count.index + 1}" }
}

# NAT Gateway so ECS tasks in private subnets can reach ECR / SNS / SQS
resource "aws_eip" "nat" {
  domain = "vpc"
  tags   = { Name = "order-service-nat-eip" }
}

resource "aws_nat_gateway" "main" {
  allocation_id = aws_eip.nat.id
  subnet_id     = aws_subnet.public[0].id
  tags          = { Name = "order-service-nat" }
  depends_on    = [aws_internet_gateway.main]
}

# Public route table → IGW
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
  tags = { Name = "order-service-public-rt" }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Private route table → NAT Gateway
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main.id
  }
  tags = { Name = "order-service-private-rt" }
}

resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}
