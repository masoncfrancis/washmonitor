FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./cmd/worker

FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=builder /worker /worker

EXPOSE 8005

ENTRYPOINT ["/worker"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://localhost:8005/health || exit 1
