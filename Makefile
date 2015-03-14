all: takeown

usage.go: README.md gendoc.py
	python gendoc.py

takeown: usage.go *.go
	go build

gofmt:
	for f in *.go; do echo gofmt -w $$f; done

.PHONY: gofmt all
