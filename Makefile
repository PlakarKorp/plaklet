GO =	go

all: test
test: check

check:
	${GO} vet ./...
	${GO} test ./...

.PHONY: all test check
