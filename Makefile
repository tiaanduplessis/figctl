BINARY  := figctl
MODULE  := github.com/tiaanduplessis/figctl
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(MODULE)/internal/cli.version=$(VERSION) \
	-X $(MODULE)/internal/cli.commit=$(COMMIT) \
	-X $(MODULE)/internal/cli.date=$(DATE)

GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

.PHONY: build test lint fmt vet vuln docs gen verify-gen check clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	@unformatted="$$(gofmt -l $(GOFILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: files need formatting:"; echo "$$unformatted"; exit 1; \
	fi
	@unimported="$$(goimports -l -local $(MODULE) $(GOFILES))"; \
	if [ -n "$$unimported" ]; then \
		echo "goimports: files need formatting:"; echo "$$unimported"; exit 1; \
	fi

vet:
	go vet ./...

vuln:
	govulncheck ./...

docs:
	go run ./cmd/gen-docs

# gen regenerates the skill reference files that are embedded in the
# binary. Run it after changing a command, a flag, or an output type.
gen:
	go run ./cmd/gen-skill-docs

# verify-gen fails when the committed skill reference files are stale.
# The same assertion runs as a unit test, so check does not repeat it.
verify-gen:
	go run ./cmd/gen-skill-docs -check

check: fmt vet lint test build

clean:
	rm -rf bin dist coverage.out
