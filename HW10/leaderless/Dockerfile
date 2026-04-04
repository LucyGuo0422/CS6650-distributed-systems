FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o server ./leaderless/

FROM alpine:latest
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
