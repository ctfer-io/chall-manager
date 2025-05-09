---
title: Software Development Kit
description: Sometimes, you don't need big things. The SDK makes sure you don't need to be a DevOps.
categories: [Explanations]
tags: [SDK, Kubernetes]
---

When you (a ChallMaker) want to deploy a single container specific for each [source](/docs/chall-manager/glossary#source), you don't want to understand how to deploy it to a specific provider. In fact, your technical expertise does not imply you are a Cloud expert... And it was not to expect !
Writing a 500-lines long [scenario](/docs/chall-manager/glossary#scenario) fitting the API only to deploy a container in a hardened environment is a tedious job you don't have time for.

For this reason, we built a Software Development Kit to ease your use of chall-manager.
It contains all the features of the chall-manager without passing you the issues of API compliance.

Additionnaly, we prepared some common use-cases factory to help you _focus on your CTF, not the infrastructure_:
- [Kubernetes ExposedMonopod](#kubernetes-exposedmonopod)
- [Kubernetes ExposedMultipod](#kubernetes-exposedmultipod)

The community is **free to create and distribute new (or alternatives) pre-made recipes**, and we welcome contributions to add new official ones. Please open an issue as a Request For Comments, and a Pull Request if possible to propose an implementation.

## Build scenarios

The common API for chall-manager scenario is very simple, defined per inputs and outputs.
They could be respectively fetched from the stack configuration and exported through stack outputs.

### Inputs

| Name | Required | Description |
|---|:---:|---|
| `identity` | ✅ | The [identity](/docs/chall-manager/glossary#identity) of the Challenge on Demand request. |

### Outputs

| Name | Required | Description |
|---|:---:|---|
| `connection_info` | ✅ | The connection information, as a string (e.g. `curl http://a4...d6.my-ctf.lan`) |
| `flag` | ❌ | The identity-specific flag the CTF platform should only validate for the given [source](/docs/chall-manager/glossary#source) |

## Kubernetes ExposedMonopod

**Fit:** deploy a single container on a Kubernetes cluster.

The `kubernetes.ExposedMonopod` helps you deploy it as a single Pod in a Deployment, expose it with 1 Service per port and if requested 1 Ingress per port.

{{< imgproc kubernetes-exposedmonopod Fit "800x800" >}}
The Kubernetes ExposedMonopod architecture for deployed resources.
{{< /imgproc >}}

The following is an example from the [24h IUT 2023](https://github.com/pandatix/24hiut-2023-cyber) usage of this SDK resource such that it deploys the Docker image `pandatix/license-lvl1:latest`, and expose port `8080` (implicitely using TCP) through an ingress. The result is used to create the connection information i.e. a `curl` example command.
For more info on configuration, please refer to the [code base](https://github.com/ctfer-io/chall-manager/blob/main/sdk/kubernetes/exposed-monopod.go).

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

## Kubernetes ExposedMultipod

**Fit:** deploy a network of containers on a Kubernetes cluster.

The Kubernetes `ExposedMultipod` helps you deploy many pods with as many deployments, services for each port of each container, ingress whenever required. It is a generalization of the [Kubernetes ExposedMonopod](#kubernetes-exposedmonopod).

{{< imgproc kubernetes-exposedmultipod Fit "800x800" >}}
The Kubernetes ExposedMultipod architecture for deployed resources.
{{< /imgproc >}}

The following is an example from the [NoBrackets 2024](https://github.com/nobrackets-ctf/NoBrackets-2024) usage of this SDK resource such that it deploys the web _vip-only_ challenge from [Drahoxx](https://x.com/50mgDrahoxx). It is composed of a NodeJS service and a MongoDB. The first is exposed through an ingress, while the other remains internal. The single rule enables traffic from the first to the second on port `27017` (implicitely using TCP).

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
