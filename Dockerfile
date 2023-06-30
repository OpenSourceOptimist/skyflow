FROM docker.io/golang:1.20.2-alpine3.17 as builder

WORKDIR /go/src/github.com/OpenSourceOptimist/skyflow/
COPY cmd ./cmd
COPY internal ./internal
COPY go.mod ./
COPY go.sum ./
COPY vendor ./vendor

RUN go build -o skyflow cmd/main.go

FROM registry.fedoraproject.org/fedora-minimal:37

COPY --from=builder /go/src/github.com/OpenSourceOptimist/skyflow/skyflow ./

CMD ["./skyflow"]
