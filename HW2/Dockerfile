# syntax=docker/dockerfile:1

FROM golang:1.24

# Set destination for COPY
WORKDIR /app

# Copy the Go Modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /docker-gs-ping

EXPOSE 8080

# Run the application
CMD ["/docker-gs-ping"]