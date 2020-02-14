default: install

build:
	go build ./...

install: build
	go install ./...

check: build
	go test ./...

clean:
	go clean

.PHONY: default build install check clean
