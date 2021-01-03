SCALER_TAG ?= unstable
SCALER_REPOSITORY ?= iguazio/

GOPATH ?= $(shell go env GOPATH)

ensure-gopath:
ifndef GOPATH
	$(error GOPATH must be set)
endif

.PHONY: build
build: dlx autoscaler
	@echo Done.

.PHONY: dlx
dlx:
	docker build -f dlx/Dockerfile --tag=$(SCALER_REPOSITORY)dlx:$(SCALER_TAG) .

.PHONY: autoscaler
autoscaler:
	docker build -f autoscaler/Dockerfile --tag=$(SCALER_REPOSITORY)autoscaler:$(SCALER_TAG) .

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
	  	(curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.33.0)

	@echo Verifying imports...

	@echo IMPI is at $(shell echo "$$GOPATH/bin/impi")

	@echo IMPI contents $(shell stat "$$GOPATH/bin/impi")

	$(GOPATH)/bin/impi \
		--local github.com/v3io/app-resource-scaler/ \
		--scheme stdLocalThirdParty \
		./...

	@echo Linting...
	$(GOPATH)/bin/golangci-lint run -v
	@echo Done.
