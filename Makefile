.PHONY: all run screenshots test race vet fmt check

run:
	go run ./cmd/player

screenshots:
	go run ./cmd/render-samples

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

all: check
	go build ./...

check:
	@unformatted=$$(gofmt -l cmd internal) || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' 'Go files need formatting:' "$$unformatted" "Run 'make fmt' to format them." >&2; \
		exit 1; \
	fi
	go test ./...
	go test -race ./...
	go vet ./...
