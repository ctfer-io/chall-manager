.PHONY: tests
tests:
	@echo "--- Unitary tests ---"
	go test ./... -run=^Test_U_ -json | tee -a gotest.json

.PHONY: buf
buf:
	buf build
	buf generate

.PHONY: update-swagger
update-swagger:
	./hack/update-swagger.sh
