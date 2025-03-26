---
title: Deployment
description: Learn to deploy Chall-Manager, either for production or development purposes.
weight: 1
categories: [How-to Guides]
tags: [Kubernetes, Infrastructure, Infra as Code]
resources:
- src: "**.png"
---

You can deploy the Chall-Manager in many ways.
The following table summarize the properties of each one.

| Name | Maintained | Isolation | Scalable | Janitor |
|---|:---:|:---:|:---:|:---:|
| [Kubernetes](#kubernetes) | ✅ | ✅ | ✅ | ✅ |
| [Binary](#binary) | ⛏️ | ❌¹ | ❌ | ✅ |
| [Docker](#docker) | ❌ | ✅ | ✅² | ✅ |

- ✅ Supported
- ❌ Unsupported
- ⛏️ Work In Progress...

¹ We do not harden the configuration in the installation script, but recommend you digging into it more as your security model requires it (especially for production purposes).

² Autoscaling is possible with an hypervisor (e.g. Docker Swarm).

## Kubernetes

{{< alert title="Note" color="primary" >}}
We **highly recommend** the use of this deployment strategy.

We use it to [test the chall-manager](/docs/chall-manager/design/testing), and will ease parallel deployments.
{{< /alert >}}

This deployment strategy guarantee you a valid infrastructure regarding our functionalities and security guidelines.
Moreover, if you are afraid of Pulumi you'll have trouble [creating scenarios](/docs/chall-manager/challmaker-guides/create-scenario), so it's a good place to start !

The requirements are:
- a distributed block storage solution such as [Longhorn](https://longhorn.io), if you want replicas.
- an [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/), if you want telemetry data.

```bash
# Get the repository and its own Pulumi factory
git clone git@github.com:ctfer-io/chall-manager.git
cd chall-manager/deploy

# Use it straightly !
# Don't forget to configure your stack if necessary.
# Refer to Pulumi's doc if necessary.
pulumi up
```

Now, you're done !

{{< imgproc infrastructure Fit "800x800" >}}
Micro Services Architecture of chall-manager deployed in a Kubernetes cluster.
{{< /imgproc >}}

## Binary

{{< alert title="Security" color="warning" >}}
We highly discourage the use of this mode for production purposes, as it does not guarantee proper isolation.
The chall-manager is basically a RCE-as-a-Service carrier, so if you run this on your host machine, prepare for dramatic issues.
{{< /alert >}}

To install it on a host machine as systemd services and timers, you can run the following script.

```bash
curl -fsSL https://github.com/ctfer-io/chall-manager/blob/main/hack/setup.sh |  sh
```

It requires:
- [`jq`](https://github.com/jqlang/jq)
- [`slsa-verifier`](https://github.com/slsa-framework/slsa-verifier)
- a privileged account

**Don't forget** that chall-manager requires [Pulumi to be installed](https://www.pulumi.com/docs/iac/download-install/).

## Docker

If you are unsatisfied of the way [the binary install](#binary) works on installation, unexisting update mecanisms or isolation, the Docker install may fit your needs.

To deploy it using Docker images, you can use the official images:
- [`ctferio/chall-manager`](https://hub.docker.com/repository/docker/ctferio/chall-manager/general)
- [`ctferio/chall-manager-janitor`](https://hub.docker.com/repository/docker/ctferio/chall-manager-janitor/general)

You can verify their integrity using the following commands.

```bash
slsa-verifier slsa-verifier verify-image "ctferio/chall-manager:<tag>@sha256:<digest>" \
    --source-uri "github.com/ctfer-io/chall-manager" \
    --source-tag "<tag>"

slsa-verifier slsa-verifier verify-image "ctferio/chall-manager-janitor:<tag>@sha256:<digest>" \
    --source-uri "github.com/ctfer-io/chall-manager" \
    --source-tag "<tag>"
```

We let the reader deploy it as needed, but recommend you take a look at how we use systemd services and timers in the [binary `setup.sh`](https://github.com/ctfer-io/chall-manager/blob/main/hack/setup.sh) script.

Additionally, we recommend you create a specific network to isolate the docker images from other adjacent services.

For instance, the following `docker-compose.yml` may fit your development needs, with the support of the janitor.

```yaml
version: '3.8'

services:
  chall-manager:
    image: ctferio/chall-manager:v0.2.0
    ports:
      - "8080:8080"

  chall-manager-janitor:
    image: ctferio/chall-manager-janitor:v0.2.0
    environment:
      URL: chall-manager:8080
      TICKER: 1m
```
