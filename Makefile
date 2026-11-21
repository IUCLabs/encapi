.PHONY: build build-server test test-short lint fmt e2e docker compose clean tidy check-go patch minor major

MODULE  := github.com/IUCLabs/encapi
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")

GO_VERSION_REQUIRED := 1.23
GO_VERSION_ACTUAL := $(shell go version | sed 's/go version go\([0-9]*\.[0-9]*\).*/\1/')

check-go:
	@if [ "$$(printf '%s\n%s' "$(GO_VERSION_REQUIRED)" "$(GO_VERSION_ACTUAL)" | sort -V | head -1)" != "$(GO_VERSION_REQUIRED)" ]; then \
		echo "ERROR: Go >= $(GO_VERSION_REQUIRED) required, found $(GO_VERSION_ACTUAL)"; exit 1; \
	fi

build: build-server

build-server: check-go tidy
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/encapi ./cmd/encapi

# Full suite, including the Postgres testcontainers integration tests (needs Docker).
test: check-go
	TESTCONTAINERS_RYUK_DISABLED=true go test -race -count=1 ./...

# Fast suite: skips the database package's container-backed tests.
test-short: check-go
	go test -race -count=1 ./internal/auth/... ./internal/config/... ./internal/enc/... ./internal/distro/... ./internal/server/... ./migrations/... ./pkg/...

lint: check-go
	go vet ./...

fmt: check-go
	gofmt -w .

docker:
	docker build -t encapi:$(VERSION) .

compose:
	docker compose up -d

clean:
	rm -rf bin/

tidy:
	go mod tidy

DEFAULT_BRANCH ?= main

# Publish the release with generated notes, then push the tag; no workflow
# fills the release, so it is not drafted.
patch minor major:
	@git fetch --quiet --tags origin $(DEFAULT_BRANCH)
	@test "$$(git branch --show-current)" = "$(DEFAULT_BRANCH)" || { echo "ERROR: not on $(DEFAULT_BRANCH)"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "ERROR: working tree not clean"; exit 1; }
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/$(DEFAULT_BRANCH))" || { echo "ERROR: HEAD is not origin/$(DEFAULT_BRANCH)"; exit 1; }
	@LATEST=$$(git tag --list 'v*' --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); \
	NEW=$$(echo "$${LATEST:-v0.0.0}" | sed 's/^v//' | awk -F. -v b=$@ '{ \
		if (b == "major") print "v" ($$1+1) ".0.0"; \
		else if (b == "minor") print "v" $$1 "." ($$2+1) ".0"; \
		else print "v" $$1 "." $$2 "." ($$3+1) }'); \
	SHA=$$(git rev-parse HEAD); \
	gh release create "$$NEW" --target "$$SHA" --generate-notes --title "$$NEW" || exit 1; \
	git push origin "$$SHA:refs/tags/$$NEW" || { gh release delete "$$NEW" --yes; exit 1; }
