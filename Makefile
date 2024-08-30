build:
	@go build -o bin/fs

run:
	@go run ./cmd/

test:
	@go test ./... -v

