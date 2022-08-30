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
# Build stage: builds the autoscaler binary
#

FROM golang:1.14.3-alpine3.11 as builder

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
COPY --from=quay.io/v3io/autoscaler:v0.4.3 /home/v3io/bin/autoscaler /home/app_resourcescaler/autoscaler

CMD ["autoscaler"]
