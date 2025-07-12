#!/bin/bash

CGO_ENABLED=0 go build -o main main.go

cp Pulumi.yaml Pulumi.yaml.bkp
yq -i '.runtime = {"name": "go", "options": {"binary": "./main"}}' Pulumi.yaml

oras push --insecure \
  "${REGISTRY}examples/no-sdk:latest" \
  --artifact-type application/vnd.ctfer-io.scenario \
  main:application/vnd.ctfer-io.file \
  Pulumi.yaml:application/vnd.ctfer-io.file

rm main
mv Pulumi.yaml.bkp Pulumi.yaml
