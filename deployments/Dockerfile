FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /juan-niang-neo ./cmd/server/

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
COPY --from=builder /juan-niang-neo /usr/local/bin/juan-niang-neo
COPY --from=builder /app/config /etc/juan-niang-neo/config
COPY --from=builder /app/data /var/lib/juan-niang-neo/data

EXPOSE 8081 8090

ENV DB_HOST=postgres \
    DB_PORT=5432 \
    DB_USER=postgres \
    DB_PASSWORD=postgres \
    DB_NAME=juan \
    REDIS_ADDR=redis:6379 \
    REDIS_PASSWORD=root \
    OB_PORT=8081 \
    API_ADDR=:8090

ENTRYPOINT ["juan-niang-neo"]
