

build: cmd internal
	go build -o skyflow cmd/main.go


make test-unit:
	go test ./internal/...
