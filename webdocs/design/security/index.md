---
title: Security
description: Learn how we designed security in a "RCE-as-a-Service" system, and how we used its features for security purposes.
weight: 9
categories: [Explanations]
tags: [Anticheat, Kubernetes]
resources:
- src: "**.png"
math: true
---

The problem with the [genericity](/docs/chall-manager/design/genericity) of chall-manager resides in its capability to execute any Golang code as long as it fits in a Pulumi stack i.e. anything. For this reason, there are multiple concerns to address when using chall-manager.

Nevertheless, it also provides actionable responses to security concerns, such as [shareflag](#shareflag) and [bias](#challops-bias).

## Authentication & Authorization

A question regarding such a security concern of an "RCE-as-a-Service" system is to throw authentication and authorization at it. Technically, it could fit and be completly justified.

Nevertheless, we think that chall-manager should not be exposed to end users and untrusted services thus [Ops](/docs/chall-manager/glossary#ops) should put mTLS in place between trusted services and restraint communications to the bare minimum. Moreover, the [Separation of Concerns Principle](https://en.wikipedia.org/wiki/Separation_of_concerns) imply authentication and authorization are another goal thus should be achieved by another service.

Finally, authentication and authorization may but justifiable if Chall-Manager was operated as a Service (in the meaning of being an online platform). As this would not be the case with a Community Edition, we consider it out of scope.

## Kubernetes

If deployed as part of a Kubernetes cluster, with a `ServiceAccount` and a specific namespace to deploy instances, Chall-Manager is able to mutate architectures on the fly. To minimize the effect of such mutations, we recommend you provide this `ServiceAccount` a `Role` with a limited set of verbs on api groups. Pay attention to minimize these rules, especially with a focus on **namespaced** resources such that it avoid undesirable side-effects.

To build this Role for your needs, you can use the command `kubectl api-resources –-namespaced=true –o wide` to visualize a cluster resources and applicable verbs.

{{< imgproc kubectl Fit "800x800" >}}
An extract of a the resources of a Kubernetes cluster and their applicable verbs.
{{< /imgproc >}}

More details on [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).

## Shareflag

One of the actionable response provided by Chall-Manager is through an anti-shareflag mecanism.

Each instance deployed by the chall-manager can return in its [scenario](/docs/chall-manager/glossary#scenario) a specific flag. This flag will then be used by the upstream CTF platform to ensure the source -and only it- found the solution.

Moreover, each instance-specific flag could be derived from an original constant one using the [flag variation engine](/docs/chall-manager/design/software-development-kit#flag-variation-engine), easing the adoption of such approach.

## ChallOps bias

As each instance is an infrastructure in itself, variations could bias them: lighter network policies, easier brute force, etc.
A [scenario](/docs/chall-manager/glossary#scenario) is not biased by essence.

If we make a risk analysis on the chall-manager capabilities and possibilities for an event, we have to consider a biased [ChallMaker](/docs/chall-manager/glossary#challmaker) or [Ops](/docs/chall-manager/glossary#ops) that produces uneven-balanced scenarios.

For this reason, the chall-manager API does not expose the source identifier of the request to the [scenario](/docs/chall-manager/glossary#scenario) code, but an [identity](/docs/chall-manager/glossary#identity). This is declined as follows. It strictly identifies an infrastructure identifier, the challenge the instance was requested from, and the source identifier for this instance.

{{< imgproc views Fit "800x800" >}}
A visualization of how views are split apart to avoid the ChallOps bias.
{{< /imgproc >}}

Notice the identity is limited to 16 hexadecimals, making it compatible to multiple uses like a DNS name or a [PRNG](https://en.wikipedia.org/wiki/Pseudorandom_number_generator) seed. This increases the possibilities of collisions, but can still cover \\(16^{16} = 18.446.744.073.709.551.616\\) combinations, trusted sufficient for a CTF (\\(f(x,y) = x \times y - 16^{16}\\), find roots: \\(x \times y=16^{16} \Leftrightarrow y=\frac{16^{16}}{x}\\) so roots are given by the couple \\((x, \frac{16^{16}}{x})\\) with \\(x\\\) the number of challenges. With largely enough challenges e.g. 200, there is still place for \\(\frac{16^{16}}{200} \simeq 9.2 \times 10^{16}\\) instances each).

## What's next ?

What about the infrastructure footprint of a production-ready deployment ?
Learn how we dealt with [resources expirations](/docs/chall-manager/design/expiration).
