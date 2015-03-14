gofmt:
	for f in *.go; do echo gofmt -w $$f; done

.PHONY: gofmt