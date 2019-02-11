SCALER_TAG ?= unstable
SCALER_REPOSITORY ?= iguazio/

.PHONY: build
build: dlx autoscaler
	@echo Done.

.PHONY: dlx
dlx:
	docker build -f dlx/Dockerfile --tag=$(SCALER_REPOSITORY)dlx:$(SCALER_TAG) .

.PHONY: autoscaler
autoscaler:
	docker build -f autoscaler/Dockerfile --tag=$(SCALER_REPOSITORY)autoscaler:$(SCALER_TAG) .
