SCALER_TAG ?= unstable
SCALER_REPOSITORY ?= iguazio/
V3IO_SCALER_TAG ?= v0.4.3
GOPATH ?= $(shell go env GOPATH)
OS_NAME = $(shell uname)

ensure-gopath:
ifndef GOPATH
	$(error GOPATH must be set)
endif

.PHONY: build
build: dlx autoscaler
	@echo Done.

.PHONY: dlx
dlx:
	docker build \
		-f dlx/Dockerfile \
		--build-arg V3IO_SCALER_TAG=$(V3IO_SCALER_TAG) \
		--tag=$(SCALER_REPOSITORY)dlx:$(SCALER_TAG) .

.PHONY: autoscaler
autoscaler:
	docker build \
		-f autoscaler/Dockerfile \
		--build-arg V3IO_SCALER_TAG=$(V3IO_SCALER_TAG) \
		--tag=$(SCALER_REPOSITORY)autoscaler:$(SCALER_TAG) .

.PHONY: modules
modules: ensure-gopath
	@echo Getting go modules
	@go mod download

.PHONY: fmt
fmt:
	gofmt -s -w .

.PHONY: lint
lint: modules
	@echo Installing linters...
	@test -e $(GOPATH)/bin/impi || \
		(mkdir -p $(GOPATH)/bin && \
		curl -s https://api.github.com/repos/pavius/impi/releases/latest \
		| grep -i "browser_download_url.*impi.*$(OS_NAME)" \
		| cut -d : -f 2,3 \
		| tr -d \" \
		| wget -O $(GOPATH)/bin/impi -qi - \
		&& chmod +x $(GOPATH)/bin/impi)

	@test -e $(GOPATH)/bin/golangci-lint || \
	  	(curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.41.1)

	@echo Verifying imports...
	$(GOPATH)/bin/impi \
		--local github.com/v3io/app-resource-scaler/ \
		--scheme stdLocalThirdParty \
		./...

	@echo Linting...
	$(GOPATH)/bin/golangci-lint run -v
	@echo Done.
