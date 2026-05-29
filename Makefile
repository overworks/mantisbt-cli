BINARY  := mantisbt-cli
VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

# os/arch pairs to cross-compile for release.
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

.PHONY: build test vet fmt clean release

build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin dist

# Cross-compile a binary for every platform in PLATFORMS into dist/.
release: clean
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=dist/$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags '$(LDFLAGS)' -o $$out . || exit 1; \
	done
