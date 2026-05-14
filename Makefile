.PHONY: build test vet lint fixtures clean

BIN := bin/sandbox

build: fixtures
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/sandbox

fixtures:
	mkdir -p embedded/lib-agent-prompt
	cp -r internal/schemas/testdata/lib-agent-prompt/* embedded/lib-agent-prompt/

test:
	go test ./...

test-integration:
	go test -tags=integration ./tests/integration/...

test-conformance:
	go test -tags=conformance ./tests/conformance/...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ embedded/
