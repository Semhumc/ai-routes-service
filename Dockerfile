# Start from the latest golang base image
FROM golang:1.20-alpine AS builder
WORKDIR /app
COPY . .
RUN cd cmd && go build -o /ai-routes-service main.go

# Start a new stage from scratch
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /ai-routes-service .
EXPOSE 8084
CMD ["./ai-routes-service"] 