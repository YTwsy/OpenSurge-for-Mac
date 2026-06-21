.PHONY: test build doctor status

test:
	go test ./...

build:
	go build -o bin/omg ./cmd/omg

doctor:
	go run ./cmd/omg doctor --config examples/config.example.yaml

status:
	go run ./cmd/omg status --config examples/config.example.yaml
