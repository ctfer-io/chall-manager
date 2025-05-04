---
title: Integrate with a CTF platform
description: Tips, tricks and guidelines on the integration of Chall-Manager in a CTF platform.
resources:
- src: "**.png"
---

So you want to integrate chall-manager with a third-party ? Good job, you are contributing to the ecosystem !

Here are the known integrations:
- **CTFd**: [ctfer-io/ctfd-chall-manager](https://github.com/ctfer-io/ctfd-chall-manager)

## The design

The API is split in two services:
- the `ChallengeStore` to handle the CRUD operations on challenges (ids, [scenarios](/docs/chall-manager/glossary#scenario), etc.).
- the `InstanceManager` to handle [players](/docs/chall-manager/dicsussions/glossary#player) CRUD operations on [instances](/docs/chall-manager/glossary#instance).

You'll have to handle both services as part of the chall-manager API if you want proper integration.

We encourage you to add additional efforts around this integration, with for instance:
- management views to monitor which challenge is started for every players ;
- pre-provisionning and/or pools to better handle load spikes at the beginning of the event ;
- add rate limiting through a [mana](/docs/ctfd-chall-manager/discussions/how-mana-works/) ;
- the support of OpenTelemetry for distributed tracing, that could ease understanding production workloads and debugging the distributed systems.

## Use the proto

Chall-Manager was conceived using Model-Based Systems Engineering, so models were first designed then manually translated (MTT) into `.proto` pseudo-code. This pseudo-code then generates the API code (TTT).

This makes the `.proto` files the perfect candidate to communicate with Chall-Manager.
Those could be found in the subdirectories [here](https://github.com/ctfer-io/chall-manager/tree/main/api/v1). Refer the your proto-to-code tool for generating a client from those.

If you are using Golang, you can directly use the generated clients for the [`ChallengeStore`](https://github.com/ctfer-io/chall-manager/blob/main/api/v1/challenge/challenge_grpc.pb.go) and [`InstanceManager`](https://github.com/ctfer-io/chall-manager/blob/main/api/v1/instance/instance_grpc.pb.go) services API.

If you cannot or don't want to use the proto files, you can [use the HTTP gateway](#use-the-gateway).

## Use the gateway

Because some languages don't support gRPC, or you don't want to, you can simply communicate with chall-manager through its JSON REST API.

Chall-Manager has a gRPC+HTTP API, so you don't have to update deployment to use it.
Nevertheless, for development purposes, we recommend you use the swagger that could be turned on with either `--gw-swagger` as an arg or `GATEWAY_SWAGGER=true` as a varenv.

You can then reach the Swagger at `/swagger`, which should show you the following.

{{< imgproc swagger Fit "800x800" >}}
The chall-manager REST JSON API Swagger.
{{< /imgproc >}}

Use this Swagger to understand the API, and build your language-specific client in order to integrate chall-manager.
We do not provide official language-specific REST JSON API clients.
