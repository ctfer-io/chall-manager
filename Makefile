.PHONY: unit-tests
unit-tests:
	@echo "--- Unitary tests ---"
	go test ./... -run=^Test_U_ -json -cover -coverprofile=cov.out | tee -a gotest.json

.PHONY: buf
buf:
	buf build
	buf generate

.PHONY: update-swagger
update-swagger:
	./hack/update-swagger.sh

TAG?=dev
.PHONY: docker
docker:
	docker build -t $(REGISTRY)ctferio/chall-manager:$(TAG) -f Dockerfile.chall-manager .
	docker push $(REGISTRY)ctferio/chall-manager:$(TAG)

	docker build -t $(REGISTRY)ctferio/chall-manager-janitor:$(TAG) -f Dockerfile.chall-manager-janitor .
	docker push $(REGISTRY)ctferio/chall-manager-janitor:$(TAG)
