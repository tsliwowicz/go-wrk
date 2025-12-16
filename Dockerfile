# syntax=docker/dockerfile:1

FROM golang:1.21 AS builder
WORKDIR /src

# Pre-download dependencies for layer caching
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/go-wrk .

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /out/go-wrk /usr/local/bin/go-wrk

ENTRYPOINT ["/usr/local/bin/go-wrk"]
CMD ["-help"]
