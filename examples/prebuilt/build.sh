#!/bin/bash

CGO_ENABLED=0 go build -o main ../teeworlds/main.go

oras push --insecure \
  "${REGISTRY}examples/prebuilt:latest" \
  --artifact-type application/vnd.ctfer-io.scenario \
  main:application/vnd.ctfer-io.file \
  Pulumi.yaml:application/vnd.ctfer-io.file

rm main
