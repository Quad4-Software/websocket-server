# Stage 1: Build the Go binary
FROM cgr.dev/chainguard/go:latest-dev AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server ./cmd/server

# Stage 2: Minimal runtime image
FROM cgr.dev/chainguard/wolfi-base:latest
WORKDIR /app
COPY --from=go-builder /app/server .
RUN apk add --no-cache ca-certificates

EXPOSE 8080
ENV PORT=8080
ENV HOST=0.0.0.0
ENV MODE=broadcast

USER 65532

CMD ["./server"]

