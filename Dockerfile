# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy everything
COPY . .

# Build Arguments
ARG APP_FILE

# 1. Disable CGO (Static Binary)
# 2. Print what we are building (Debug)
# 3. Build with verbose output (-v)
RUN echo "Building target: $APP_FILE" && \
    CGO_ENABLED=0 go build -o binary -v $APP_FILE

# Run Stage
FROM alpine:latest

WORKDIR /root/
COPY --from=builder /app/binary .

EXPOSE 8080 8081 6060 7777

CMD ["./binary"]
