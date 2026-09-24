.PHONY: run test race vet fmt check

run:
	go run ./cmd/player

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

check:
	test -z "$$(gofmt -l $$(find cmd internal -name '*.go'))"
	go test ./...
	go test -race ./...
	go vet ./...
