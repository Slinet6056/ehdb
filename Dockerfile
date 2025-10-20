FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git tzdata

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o ehdb-api cmd/api/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

ENV TZ=Asia/Shanghai

RUN addgroup -g 1000 ehdb && \
    adduser -D -u 1000 -G ehdb ehdb

WORKDIR /app

COPY --from=builder /build/ehdb-api .

COPY config.example.yaml ./

RUN chown -R ehdb:ehdb /app

USER ehdb

EXPOSE 8880

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8880/ || exit 1

CMD ["./ehdb-api", "-config", "/app/config.yaml", "-scheduler"]
