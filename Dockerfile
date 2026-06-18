# Stage 1: Build the application and compile eBPF
FROM golang:1.24-bookworm AS builder

# Install build dependencies for eBPF compilation
RUN apt-get update && apt-get install -y \
    build-essential \
    clang \
    llvm \
    libbpf-dev \
    && rm -rf /var/lib/apt/lists/*

# Fix architecture specific asm headers lookup
RUN ln -s /usr/include/x86_64-linux-gnu/asm /usr/include/asm

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate eBPF code and build
RUN go generate ./...
RUN go build -o mcpwatch main.go

# Run tests
RUN go test -v ./...

# Stage 2: Final minimal runner image
FROM debian:bookworm-slim

# Install runtime dependencies for eBPF loader (libc, libbpf)
RUN apt-get update && apt-get install -y \
    libc6 \
    libbpf1 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy compiled binary from builder stage
COPY --from=builder /app/mcpwatch /app/mcpwatch

# Expose default UI and proxy ports
EXPOSE 8080 8081

ENTRYPOINT ["/app/mcpwatch"]
