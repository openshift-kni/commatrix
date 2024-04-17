.PHONY: e2etest

unit-test:
	go test ./pkg/...

e2etest:
	ginkgo e2etest
