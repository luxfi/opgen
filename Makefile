GO ?= go

.PHONY: all build test check fmt vet clean

all: check

build:
	$(GO) build ./...

# The compile checks build the generated Rust, C++ and Go for real. They need
# cargo, a C++20 compiler and nlohmann/json, and skip themselves when one is
# missing — so `make test` is honest about what it proved.
test:
	$(GO) test ./...

check: fmt vet test

fmt:
	@test -z "$$(gofmt -l . )" || { gofmt -l .; echo "gofmt"; exit 1; }

vet:
	$(GO) vet ./...

clean:
	$(GO) clean ./...
