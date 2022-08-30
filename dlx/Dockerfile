# Copyright 2019 Iguazio
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
#
# Build stage: builds the dlx binary
#

ARG V3IO_SCALER_TAG
FROM gcr.io/iguazio/golang:1.17-alpine3.15 as builder

RUN apk --update --no-cache add \
    git \
    gcc \
    musl-dev

WORKDIR /app_resourcescaler

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN mkdir -p /home/app_resourcescaler/bin \
        && GOOS=linux GOARCH=amd64 go build -buildmode=plugin -a -installsuffix cgo -ldflags="-s -w" \
        -o /home/app_resourcescaler/plugins/plugin.so ./resourcescaler.go \
        && rm -rf /app_resourcescaler

FROM quay.io/v3io/dlx:$V3IO_SCALER_TAG as source

FROM gcr.io/iguazio/alpine:3.16

WORKDIR /home/app_resourcescaler

ENV PATH=/home/app_resourcescaler:$PATH

COPY --from=builder /home/app_resourcescaler/plugins/plugin.so /home/app_resourcescaler/plugins/plugin.so
COPY --from=source /home/v3io/bin/dlx /home/app_resourcescaler/dlx

CMD ["dlx"]
