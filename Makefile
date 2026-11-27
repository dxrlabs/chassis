.PHONY: tidy test test-race build fmt vet lint ci work-sync

modules := $(shell go list -f '{{.Dir}}' -m)

work-sync:
	go work sync

tidy: work-sync
	@for m in $(modules); do \
		echo "tidy $$m"; \
		(cd $$m && go mod tidy) || exit 1; \
	done

fmt:
	gofmt -w $(shell find . -name '*.go' -not -path './.git/*')

vet:
	@for m in $(modules); do \
		echo "vet $$m"; \
		go vet $$m/... || exit 1; \
	done

lint:
	@for m in $(modules); do \
		echo "lint $$m"; \
		golangci-lint run "$$m"/... || exit 1; \
	done

test:
	@for m in $(modules); do \
		echo "test $$m"; \
		go test $$m/... -count=1 || exit 1; \
	done

test-race:
	@for m in $(modules); do \
		echo "test-race $$m"; \
		go test $$m/... -count=1 -race || exit 1; \
	done

build:
	@for m in $(modules); do \
		echo "build $$m"; \
		go build $$m/... || exit 1; \
	done

ci: work-sync fmt vet test build

ci-full: ci test-race
