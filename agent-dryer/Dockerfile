FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /agent ./cmd/agent

# Stage 2: imagen final
FROM alpine:3.20

RUN apk add --no-cache ca-certificates
COPY --from=builder /agent /agent

EXPOSE 8005

ENTRYPOINT ["/agent"]