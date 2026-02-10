FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /resemble ./cmd/resemble/

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates
COPY --from=builder /resemble /usr/local/bin/resemble
ENTRYPOINT ["resemble"]
