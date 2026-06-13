.PHONY: run build test vet lint

run:   ; go run ./cmd
build: ; go build -o bin/app ./cmd
test:  ; go test ./...
vet:   ; go vet ./...
lint:  ; golangci-lint run
