#
# Build stage: builds the dlx binary
#

FROM gcr.io/iguazio/golang:1.17-alpine3.15 as builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /app_resourcescaler

COPY . .

RUN go mod download

RUN mkdir -p /home/app_resourcescaler/bin \
        && GOOS=linux GOARCH=amd64 go build -buildmode=plugin -a -installsuffix cgo -ldflags="-s -w" \
        -o /home/app_resourcescaler/plugins/plugin.so ./resourcescaler.go \
        && rm -rf /app_resourcescaler

FROM gcr.io/iguazio/alpine:3.16

WORKDIR /home/app_resourcescaler

ENV PATH=/home/app_resourcescaler:$PATH

COPY --from=builder /home/app_resourcescaler/plugins/plugin.so /home/app_resourcescaler/plugins/plugin.so
COPY --from=quay.io/v3io/dlx:v0.4.3 /home/v3io/bin/dlx /home/app_resourcescaler/dlx

CMD ["dlx"]
