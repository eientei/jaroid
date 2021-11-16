FROM golang:1.16 as builder
WORKDIR /go/src/github.com/eientei/jaroid
COPY . .
RUN go mod download
RUN export GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPRIVATE=github.com/eientei/jaroid && \
  go install -tags netgo -ldflags '-w -extldflags "-static"' -v ./cmd/...
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/jaroid /
ENTRYPOINT ["/jaroid"]
