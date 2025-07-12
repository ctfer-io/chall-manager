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
- [Kubernetes ExposedMultipod](#kubernetes-exposedmultipod)

The community is free to create new pre-made recipes, and we welcome contributions to add new official ones. Please open an issue as a Request For Comments, and a Pull Request if possible to propose an implementation.

## Build scenarios

Fitting the chall-manager scenario API imply fitting inputs and outputs models.

Even if easy, it still requires work, and functionalities or evolutions does not guarantee you easy maintenance: offline compatibility with OCI registry, pre-configured providers, etc.

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
		emp, err := k8s.NewExposedMonopod(req.Ctx, "license-lvl1", &k8s.ExposedMonopodArgs{
			Identity: pulumi.String(req.Config.Identity),
			Hostname: pulumi.String("brefctf.ctfer.io"),
			Container: k8s.ContainerArgs{
				Image: pulumi.String("pandatix/license-lvl1:latest"),
				Ports: k8s.PortBindingArray{
					k8s.PortBindingArgs{
						Port:       pulumi.Int(8080),
						ExposeType: k8s.ExposeIngress,
					},
				},
			},
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", emp.URLs.MapIndex(pulumi.String("8080/TCP")))
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

## Kubernetes ExposedMultipod

When you want to deploy multiple containers together (e.g. a web app with a frontend, a backend, a database and a cache), on a Kubernetes cluster, and want it to be fast and easy.

Then, the Kubernetes `ExposedMultipod` fits your needs ! Your can easily configure the containers and the networking rules between them so it deploys to production in the next seconds.
The following shows you how easy it is to write a scenario that creates multiple deployments, services, ingresses, configmaps, ... and provide the connection information as a `curl` command.

{{< card code=true header="`main.go`" lang="go" >}}
package main

import (
	"github.com/ctfer-io/chall-manager/sdk"
	"github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
			emp, err := kubernetes.NewExposedMultipod(req.Ctx, "vip-only", &kubernetes.ExposedMultipodArgs{
			Identity: pulumi.String(req.Config.Identity),
			Hostname: pulumi.String("brefctf.ctfer.io"),
			Containers: kubernetes.ContainerMap{
				"node": kubernetes.ContainerArgs{
					Image: pulumi.String("pandatix/vip-only-node:latest"),
					Ports: kubernetes.PortBindingArray{
						kubernetes.PortBindingArgs{
							Port:       pulumi.Int(3000),
							ExposeType: kubernetes.ExposeIngress,
						},
					},
				},
				"mongo": kubernetes.ContainerArgs{
					Image: pulumi.String("pandatix/vip-only-mongo:latest"),
					Ports: kubernetes.PortBindingArray{
						kubernetes.PortBindingArgs{
							Port: pulumi.Int(27017),
						},
					},
				},
			},
			Rules: kubernetes.RuleArray{
				kubernetes.RuleArgs{
					From: pulumi.String("node"),
					To:   pulumi.String("mongo"),
					On:   pulumi.Int(27017),
				},
			},
		}, opts...)
		if err != nil {
			return err
		}

		resp.ConnectionInfo = pulumi.Sprintf("curl -v https://%s", emp.URLs.
			MapIndex(pulumi.String("node")).
			MapIndex(pulumi.String("3000/TCP")),
		)
		return nil
	})
}
{{< /card >}}

{{< alert title="Requirements" color="warning" >}}
To use ingresses, make sure your Kubernetes cluster can deal with them: have an ingress controller (e.g. [Traefik](https://traefik.io/)), and DNS resolution points to the Kubernetes cluster.
{{< /alert >}}

{{< imgproc kubernetes-exposedmultipod Fit "800x800" >}}
The Kubernetes ExposedMultipod architecture for deployed resources.
{{< /imgproc >}}

The ExposedMultipod is a generalization of the [ExposedMonopod](#kubernetes-exposedmonopod) with \[n\] containers. In fact, the later's implementation passes its container to the first as a network of a single container.
