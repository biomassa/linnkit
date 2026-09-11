BIN := bin/linnkit

.PHONY: build test vet fmt-check check clean

build:
	go build -o $(BIN) ./cmd/linnkit

test:
	go test ./...

vet:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

check: fmt-check vet test

clean:
	rm -rf bin
