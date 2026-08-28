MODULES  := $(notdir $(wildcard cmd/*))
GOBIN    := $(shell go env GOPATH)/bin

.PHONY: all build test lint vet docker clean
.PHONY: $(MODULES:%=build-%) $(MODULES:%=test-%) $(MODULES:%=lint-%) $(MODULES:%=vet-%) $(MODULES:%=docker-%)
.PHONY: pkgs staging integration-test contracts\:test contracts\:check

all: build test lint

build: $(MODULES:%=build-%)
	@echo "All modules built"

test: $(MODULES:%=test-%)
	@echo "All modules tested"

lint: $(MODULES:%=lint-%)
	@echo "All modules linted"

vet: $(MODULES:%=vet-%)
	@echo "All modules vetted"

docker: $(MODULES:%=docker-%)
	@echo "All Docker images built"

clean:
	$(RM) -r $(MODULES:%=cmd/%/out)

# ---- per-module targets ----

define MODULE_RULES
build-$(1):
	@echo "=== build $(1) ==="
	$$(MAKE) -C cmd/$(1) build

test-$(1):
	@echo "=== test $(1) ==="
	$$(MAKE) -C cmd/$(1) test

lint-$(1):
	@echo "=== lint $(1) ==="
	$$(MAKE) -C cmd/$(1) lint

vet-$(1):
	@echo "=== vet $(1) ==="
	$$(MAKE) -C cmd/$(1) vet

docker-$(1):
	@echo "=== docker $(1) ==="
	$$(MAKE) -C cmd/$(1) docker
endef

$(foreach m,$(MODULES),$(eval $(call MODULE_RULES,$(m))))

# ---- pkg / staging ----

pkgs:
	@for mod in pkg/*/; do echo "=== build $$mod ==="; (cd "$$mod" && go build ./...); done
	@for mod in staging/*/; do echo "=== build $$mod ==="; (cd "$$mod" && go build ./...); done

# ---- integration tests (requires PostgreSQL) ----

integration-test:
	@echo "=== integration tests ==="
	@for mod in cmd/*/; do \
		if [ -f "$$mod/Makefile" ]; then \
			$(MAKE) -C "$$mod" integration-test 2>/dev/null || true; \
		fi; \
	done

contracts\:test:
	node --test scripts/contracts.test.mjs

contracts\:check:
	npm run contracts:check

# ---- helm ----

helm-lint:
	helm lint deploy/charts/hnb

helm-template:
	helm template hnb deploy/charts/hnb
