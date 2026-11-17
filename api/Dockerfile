# Stage 1: build
FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .


RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Stage 2: imagen final
FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=builder /server /server

EXPOSE 8001

CMD ["/server"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://localhost:8001/health || exit 1