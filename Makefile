lint:
	go tool golangci-lint run

fmt:
	go fmt ./...

generate:
	go generate ./...
