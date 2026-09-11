.PHONY: test vet fmt check

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	go run mvdan.cc/gofumpt@v0.12.0 -l -w .

check:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
