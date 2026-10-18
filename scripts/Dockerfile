FROM golang:latest AS builder
WORKDIR /app
COPY go.mod ./
COPY go.sum ./
COPY ./cmd ./cmd/
COPY ./internal ./internal/
COPY ./update ./update/
COPY ./certs/ ./certs/

RUN go mod download
ARG ENV=dev
ENV ENV=${ENV}
RUN CGO_ENABLED=1 GOOS=linux go build -o main ./cmd/portable

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y \
    ca-certificates libevent-2.1-7 openssl zlib1g && \
    rm -rf /var/lib/apt/lists/*
ARG ENV=dev
# RUN apk --no-cache add ca-certificates
# RUN adduser -D -s /bin/sh appuser
WORKDIR /app
COPY --from=builder /app/main .
# RUN chown -R appuser:appuser /app
# USER appuser
ENV ENV=${ENV}
EXPOSE 3000
CMD ["./main"]