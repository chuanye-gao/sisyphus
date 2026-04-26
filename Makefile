.PHONY: build test clean install vet lint

BINARY := sisyphus
BINDIR := bin
SRCDIR := cmd/sisyphus

GO ?= go
GOFLAGS ?=

# Build the binary
build:
	$(GO) build $(GOFLAGS) -o $(BINDIR)/$(BINARY) ./$(SRCDIR)

# Run all tests with race detection
test:
	$(GO) test -race -count=1 ./...

# Run go vet
vet:
	$(GO) vet ./...

# Run go vet + staticcheck if available
lint: vet
	@which staticcheck >/dev/null 2>&1 && staticcheck ./... || true

# Clean build artifacts
clean:
	rm -rf $(BINDIR)

# Install to /usr/local/bin (requires root or sudo)
install: build
	install -m 755 $(BINDIR)/$(BINARY) /usr/local/bin/$(BINARY)

# Download dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy

# Build with optimizations for production
release:
	$(GO) build -ldflags="-s -w" -o $(BINDIR)/$(BINARY) ./$(SRCDIR)
