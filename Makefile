SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

all: build lint

build:
	go build .

lint:
	go tool golangci-lint run

fmt:
	go fmt ./...

test:
	go tool ginkgo run ./...

generate:
	go generate ./...

govulncheck:
	go tool govulncheck ./...

update-tools:
	for tool in `go list tool`; do go -C tool/`basename $${tool}` get -u tool; done
