

build: cmd internal
	go build -o skyflow cmd/main.go

test-component:
	go test -c ./test/component/...

make test-unit:
	go test ./internal/...
