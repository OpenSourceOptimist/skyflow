

build: cmd internal
	go build -o skyflow cmd/main.go

test-component:
	go test -c ./test/component/...

test-unit:
	go test ./internal/...

lint:
	golangci-lint run

benchmark:
	go build -o skyflow_bench test/benchmark/main.go
	./skyflow_bench
