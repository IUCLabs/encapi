.PHONY: build build-server build-cli test test-short lint fmt e2e docker compose clean tidy rpm rpm-package check-go patch minor major

MODULE  := github.com/IUCLabs/encapi
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
DIST    := dist
OS      ?= $(shell go env GOOS)
ARCH    ?= $(shell go env GOARCH)

# git.example.com modules (golib) are fetched straight from Gitea, never via the
# public proxy or sum db, which have no view of them. Exported here rather than
# written with `go env -w`, so a fresh checkout needs no machine-local setup.
export GOPRIVATE := git.example.com

GO_VERSION_REQUIRED := 1.23
GO_VERSION_ACTUAL := $(shell go version | sed 's/go version go\([0-9]*\.[0-9]*\).*/\1/')

check-go:
	@if [ "$$(printf '%s\n%s' "$(GO_VERSION_REQUIRED)" "$(GO_VERSION_ACTUAL)" | sort -V | head -1)" != "$(GO_VERSION_REQUIRED)" ]; then \
		echo "ERROR: Go >= $(GO_VERSION_REQUIRED) required, found $(GO_VERSION_ACTUAL)"; exit 1; \
	fi

build: build-server build-cli

build-server: check-go tidy
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/encapi ./cmd/encapi

build-cli: check-go
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(DIST)/encapi-cli ./cmd/encapi-cli

# Full suite, including the Postgres testcontainers integration tests (needs Docker).
test: check-go
	TESTCONTAINERS_RYUK_DISABLED=true go test -race -count=1 ./...

# Fast suite: skips the database package's container-backed tests.
test-short: check-go
	go test -race -count=1 ./internal/config/... ./internal/enc/... ./internal/distro/... ./internal/server/... ./internal/cli/... ./migrations/... ./pkg/...

lint: check-go
	go vet ./...

fmt: check-go
	gofmt -w .

docker:
	docker build -t encapi:$(VERSION) .

compose:
	docker compose up -d

# --- CLI RPM (encapi-cli + the puppet ENC wrapper) ---
rpm: build-cli rpm-package

rpm-package:
	./scripts/build-rpm.sh $(VERSION)

clean:
	rm -rf bin/ $(DIST)/

tidy:
	go mod tidy

_LATEST := $(shell git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1)
_BASE   := $(if $(_LATEST),$(_LATEST),v0.0.0)
_MAJ    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f1)
_MIN    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f2)
_PAT    := $(shell echo $(_BASE) | sed 's/^v//' | cut -d. -f3)

patch:
	@NEW=v$(_MAJ).$(_MIN).$(shell expr $(_PAT) + 1); \
	git tag $$NEW && echo "Tagged $$NEW" && git push origin $$NEW

minor:
	@NEW=v$(_MAJ).$(shell expr $(_MIN) + 1).0; \
	git tag $$NEW && echo "Tagged $$NEW" && git push origin $$NEW

major:
	@NEW=v$(shell expr $(_MAJ) + 1).0.0; \
	git tag $$NEW && echo "Tagged $$NEW" && git push origin $$NEW
