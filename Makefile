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

SCALER_TAG ?= unstable
SCALER_REPOSITORY ?= iguazio
GOPATH ?= $(shell go env GOPATH)
OS_NAME = $(shell uname)

ensure-gopath:
ifndef GOPATH
	$(error GOPATH must be set)
endif

.PHONY: build
build: dlx autoscaler
	@echo Done.

.PHONY: push
push:
	docker push $(SCALER_REPOSITORY)/dlx:$(SCALER_TAG)
	docker push $(SCALER_REPOSITORY)/autoscaler:$(SCALER_TAG)

.PHONY: dlx
dlx:
	docker build \
		--file cmd/dlx/Dockerfile \
		--tag $(SCALER_REPOSITORY)/dlx:$(SCALER_TAG) \
		.

.PHONY: autoscaler
autoscaler:
	docker build \
		--file cmd/autoscaler/Dockerfile \
		--tag $(SCALER_REPOSITORY)/autoscaler:$(SCALER_TAG) \
		.

.PHONY: fmt
fmt:
	gofmt -s -w .
	./hack/lint/install.sh
	.bin/golangci-lint run --fix

.PHONY: lint
lint: modules
	./hack/lint/install.sh
	./hack/lint/run.sh

.PHONY: modules
modules: ensure-gopath
	@echo Getting go modules
	@go mod download
