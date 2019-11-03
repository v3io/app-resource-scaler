#
# Build stage: builds the dlx binary
#

FROM golang:1.10-alpine3.7 as builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /go/src/github.com/v3io/app_resourcescaler

COPY . .

RUN go get github.com/v3io/scaler-types

RUN cd $GOPATH/src/github.com/v3io/scaler-types \
    && git checkout v1.0.0 \
    && cd /go/src/github.com/v3io/app_resourcescaler

RUN mkdir -p /home/app_resourcescaler/bin \
        && GOOS=linux GOARCH=amd64 go build -buildmode=plugin -a -installsuffix cgo -ldflags="-s -w" \
        -o /home/app_resourcescaler/plugins/plugin.so ./resourcescaler.go \
        && rm -rf /go/src/github.com/v3io/app_resourcescaler

FROM alpine:3.7

WORKDIR /home/app_resourcescaler

ENV PATH=/home/app_resourcescaler:$PATH

COPY --from=builder /home/app_resourcescaler/plugins/plugin.so /home/app_resourcescaler/plugins/plugin.so
COPY --from=quay.io/v3io/dlx:v0.0.1 /home/v3io/bin/dlx /home/app_resourcescaler/dlx

CMD ["dlx"]
