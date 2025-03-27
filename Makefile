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
