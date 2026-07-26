.PHONY: build test integration benchmark run token

build:
	go build -trimpath -o bin/gateway ./cmd/gateway

test:
	go test -race -cover ./...

integration:
	go test -tags=integration ./tests/integration/...

benchmark:
	go test -run=^$$ -bench=. -benchmem ./internal/limiter/...

run:
	go run ./cmd/gateway

token:
	go run ./cmd/token demo-user user
