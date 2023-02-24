FROM registry.fedoraproject.org/fedora-minimal:37 as builder

RUN microdnf install go -y

WORKDIR /go/src/github.com/OpenSourceOptimist/skyflow/
COPY cmd ./cmd
COPY internal ./internal
COPY go.mod ./
COPY go.sum ./
COPY vendor ./vendor

RUN ls

RUN go build -o skyflow cmd/main.go

FROM registry.fedoraproject.org/fedora-minimal:37

COPY --from=builder /go/src/github.com/OpenSourceOptimist/skyflow/skyflow ./

CMD ["./skyflow"]
