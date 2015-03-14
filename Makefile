all: cmd/takeown/takeown

cmd/takeown/usage.go: README.md build/gendoc.py
	python build/gendoc.py

cmd/takeown/takeown: cmd/takeown/usage.go cmd/takeown/*.go
	cd cmd/takeown && go build && cd ../..

gofmt:
	for f in cmd/takeown/*.go; do echo gofmt -w $$f; done

.PHONY: gofmt all
