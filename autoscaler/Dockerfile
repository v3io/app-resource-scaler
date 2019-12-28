#
# Build stage: builds the autoscaler binary
#

FROM golang:1.10-alpine3.7 as builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /go/src/github.com/v3io/app_resourcescaler

COPY . .

RUN go get github.com/v3io/scaler-types \
    && cd $GOPATH/src/github.com/v3io/scaler-types \
    && git checkout v1.4.0 \
    && cd /go/src/github.com/v3io/app_resourcescaler

RUN mkdir -p /home/app_resourcescaler/bin \
        && GOOS=linux GOARCH=amd64 go build -buildmode=plugin -a -installsuffix cgo -ldflags="-s -w" \
        -o /home/app_resourcescaler/plugins/plugin.so ./resourcescaler.go \
        && rm -rf /go/src/github.com/v3io/app_resourcescaler

FROM alpine:3.7

WORKDIR /home/app_resourcescaler

ENV PATH=/home/app_resourcescaler:$PATH

COPY --from=builder /home/app_resourcescaler/plugins/plugin.so /home/app_resourcescaler/plugins/plugin.so
COPY --from=quay.io/v3io/autoscaler:v0.1.2 /home/v3io/bin/autoscaler /home/app_resourcescaler/autoscaler

CMD ["autoscaler"]
