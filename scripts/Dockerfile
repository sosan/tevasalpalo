FROM golang:alpine AS builder
# FROM bitnami/golang:latest AS builder
RUN apk add git ca-certificates gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
COPY *.* ./
COPY update ./update/
COPY assets/ ./assets/
COPY certs/ ./certs/
# COPY runtime/ ./runtime/
COPY views/ ./views/
RUN go mod download
ARG ENV=dev
ENV ENV=${ENV}
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

FROM alpine:latest
ARG ENV=dev
RUN apk --no-cache add ca-certificates
RUN adduser -D -s /bin/sh appuser
WORKDIR /app
COPY --from=builder /app/main .
RUN chown -R appuser:appuser /app
USER appuser
ENV ENV=${ENV}
EXPOSE 3000
ENTRYPOINT ["./main"]