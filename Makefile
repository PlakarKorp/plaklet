GO =	go

all: plaklet plakletcli test
test: check

check:
	${GO} vet ./...
	${GO} test ./...

plaklet:
	${GO} build -o bin/plaklet ./cmd/plaklet

plakletcli:
	${GO} build -o bin/plakletcli ./cmd/plakletcli

.PHONY: all test check plaklet plakletcli
