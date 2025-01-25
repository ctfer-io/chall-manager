---
title: Software Development Kit
description: Sometimes, you don't need big things. The SDK makes sure you don't need to be a DevOps.
categories: [Explanations]
tags: [SDK, Kubernetes]
---

When you (a ChallMaker) want to deploy a single container specific for each [source](/docs/chall-manager/glossary#source), you don't want to understand how to deploy it to a specific provider. In fact, your technical expertise does not imply you are a Cloud expert... And it was not to expect !
Writing a 500-lines long [scenario](/docs/chall-manager/glossary#scenario) fitting the API only to deploy a container is a tedious job you don't want to do more than once: create a deployment, the service, possibly the ingress, have a configuration and secrets to handle...

For this reason, we built a Software Development Kit to ease your use of chall-manager.
It contains all the features of the chall-manager without passing you the issues of API compliance.

Additionnaly, we prepared some common use-cases factory to help you _focus on your CTF, not the infrastructure_:
- [Kubernetes ExposedMonopod](#kubernetes-exposedmonopod)

The community is free to create new pre-made recipes, and we welcome contributions to add new official ones. Please open an issue as a Request For Comments, and a Pull Request if possible to propose an implementation.

## Build scenarios

Fitting the chall-manager scenario API imply inputs and outputs.

Despite it not being complex, it still requires work, and functionalities or evolutions does not guarantee you easy maintenance: offline compatibility with OCI registry, pre-configured providers, etc.

Indeed, if you are dealing with a chall-manager deployed in a Kubernetes cluster, the `...pulumi.ResourceOption` contains a pre-configured provider such that every Kubernetes resources the scenario will create, they will be deployed in the proper namespace.

### Inputs

Those are fetchable from the Pulumi configuration.

| Name | Required | Description |
|---|:---:|---|
| `identity` | ✅ | the [identity](/docs/chall-manager/glossary#identity) of the Challenge on Demand request |

### Outputs

Those should be exported from the Pulumi context.

| Name | Required | Description |
|---|:---:|---|
| `connection_info` | ✅ | the connection information, as a string (e.g. `curl http://a4...d6.my-ctf.lan`) |
| `flag` | ❌ | the identity-specific flag the CTF platform should only validate for the given [source](/docs/chall-manager/glossary#source) |

## Kubernetes ExposedMonopod

When you want to deploy a challenge composed of a single container, on a Kubernetes cluster, you want it to be fast and easy.

Then, the Kubernetes `ExposedMonopod` fits your needs ! You can easily configure the container you are looking for and deploy it to production in the next seconds.
The following shows you how easy it is to write a scenario that creates a Deployment with a single replica of a container, exposes a port through a service, then build the ingress specific to the [identity](/docs/chall-manager/glossary#identity) and finally provide the connection information as a `curl` command.

{{< card code=true header="`main.go`" lang="go" >}}
package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	"github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		cm, err := kubernetes.NewExposedMonopod(req.Ctx, &kubernetes.ExposedMonopodArgs{
			Image:      pulumi.String("myprofile/my-challenge:latest"),
			Port:       pulumi.Int(8080),
			ExposeType: kubernetes.ExposeNodePort,
			Hostname:   pulumi.String("brefctf.ctfer.io"),
			Identity:   pulumi.String(req.Config.Identity),
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v http://%s", cm.URL)
		return nil
	})
}
{{< /card >}}

{{< alert title="Requirements" color="warning" >}}
To use ingresses, make sure your Kubernetes cluster can deal with them: have an ingress controller (e.g. [Traefik](https://traefik.io/)), and DNS resolution points to the Kubernetes cluster.
{{< /alert >}}

{{< imgproc kubernetes-exposedmonopod Fit "800x800" >}}
The Kubernetes ExposedMonopod architecture for deployed resources.
{{< /imgproc >}}

<!-- TODO provide ExposedMonopod configuration (attributes, required/optional, type, description) -->
