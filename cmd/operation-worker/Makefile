BINARY   := $(notdir $(CURDIR))
OUTDIR   := ./out
GO       := go
GOFLAGS  := -trimpath -ldflags="-s -w"
CGOFLAGS := CGO_ENABLED=0

.PHONY: build test lint vet docker clean

build: $(OUTDIR)/$(BINARY)

$(OUTDIR)/$(BINARY): $(wildcard *.go) $(wildcard internal/**/*.go) go.mod go.sum
	@mkdir -p $(OUTDIR)
	$(CGOFLAGS) $(GO) build $(GOFLAGS) -o $(OUTDIR)/$(BINARY) .

test:
	$(GO) test -count=1 ./...

integration-test:
	HNB_TEST_POSTGRES_DSN="host=127.0.0.1 port=5432 dbname=hnb_test user=postgres password=test123 sslmode=disable" \
		$(GO) test -count=1 -v -run 'TestCommitStep' ./internal/store/

lint:
	$(GO) vet ./...

vet:
	$(GO) vet ./...

docker:
	docker build -t hnb/$(BINARY):latest .

clean:
	$(RM) -r $(OUTDIR)