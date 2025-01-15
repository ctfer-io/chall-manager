#!/bin/bash

CGO_ENABLED=0 go build -o main main.go
zip -r scenario.zip main Pulumi.yaml
