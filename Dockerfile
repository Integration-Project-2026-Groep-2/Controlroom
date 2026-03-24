FROM golang:1.26.1-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TODO(nasr): look for further optimzations (example: architecture optimzations)
RUN CGO_ENABLED=0 GOOS=linux go build -o controlroom ./cmd/controlroom

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/controlroom .

CMD ["./controlroom"]
