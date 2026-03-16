FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o order-service .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/order-service .
EXPOSE 8080
ENTRYPOINT ["./order-service"]
