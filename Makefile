WAILS := go tool wails

# The Go runtime needs macOS 13, and the webview and the camera both link libobjc, which the linker reports as a duplicate.
WAILS_LDFLAGS := -mmacosx-version-min=13.0 -Wl,-no_warn_duplicate_libraries
APP := build/bin/coincheur.app

.PHONY: deps ort model frontend-deps frontend dev build bundle test lint generate evaluate

deps: ort model frontend-deps

ort:
	./scripts/fetch-ort.sh

model:
	./scripts/export-model.sh

frontend-deps:
	cd frontend && npm install

frontend:
	cd frontend && npm run build

dev:
	CGO_LDFLAGS="$(WAILS_LDFLAGS)" $(WAILS) dev

build:
	CGO_LDFLAGS="$(WAILS_LDFLAGS)" $(WAILS) build
	$(MAKE) bundle

bundle:
	mkdir -p $(APP)/Contents/Frameworks $(APP)/Contents/Resources
	cp build/lib/libonnxruntime.dylib $(APP)/Contents/Frameworks/
	cp build/models/corners.onnx $(APP)/Contents/Resources/

test:
	go test -race ./...
	cd frontend && npm run typecheck

lint:
	golangci-lint run --allow-parallel-runners ./...

generate:
	go generate ./...

evaluate:
	go build -o build/evaluate ./cmd/evaluate
