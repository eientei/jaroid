FROM golang:1.16 as builder
WORKDIR /go/src/github.com/eientei/jaroid
COPY . .
RUN go mod download
RUN export GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOPRIVATE=github.com/eientei/jaroid && \
  go install -tags netgo -ldflags '-w -extldflags "-static"' -v ./cmd/...
FROM python:3.9.7-alpine3.14
RUN apk add git ffmpeg && \
  pip install --no-cache-dir git+https://github.com/animelover1984/youtube-dl.git@Niconico_download_fix && \
  pip install --no-cache-dir -r https://raw.githubusercontent.com/animelover1984/youtube-dl/Niconico_download_fix/requirements.txt && \
  pip install --no-cache-dir python-dateutil
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/jaroid /
ENTRYPOINT ["/jaroid"]
