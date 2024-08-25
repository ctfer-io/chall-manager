---
title: Genericity
description: What is the layer of genericity ?
categories: [Explanation]
weight: 3
---

While trying to find a generic approach to deploy any infrastructure with a non-vendor-lock-in API, we looked at existing approaches. None of them proposed such an API, so we had to pave the way to a new future. But we did not know how to do it.

One day, after deploying infrastructures with [Victor](https://github.com/ctfer-io/victor), we realised it was the solution. Indeed, Victor is able to deploy any Pulumi stack. This imply that the solution was already before our eyes: a Pulumi stack.

This consist the **genericity layer**, as easy as this.

## Pulumi as the solution

To go from theory to practice, we had to make choices.
One of the problem with a large genericity is it being... large, actually.

If you consider all ecosystems covered by Pulumi, to cover them you'll require all runtimes installed on the host machine.
For instance, the [Pulumi Docker image](https://hub.docker.com/r/pulumi/pulumi) is around 1.5 GB. This imply that a generic solution covering all ecosystems would be around 2 GB of memory.

Moreover, the enhancements you can propose in a language would have to be re-implemented similarly in every language, or transpiled. As [transpilation](https://en.wikipedia.org/wiki/Source-to-source_compiler) is a heavy task, either manual or automatic but with a high error rate, it is not suitable for production.

Our choice was to **focus on one language first** (Golang), and later permit transpilation in other languages if technically automatable with a high success rate.
With this choice, we would only have to deal with the [Pulumi Go Docker image](https://hub.docker.com/r/pulumi/pulumi-go), around 200 MB (a 7.5 reduction factor). It could be even more reduced using minified images, using [the Slim Toolkit](https://github.com/slimtoolkit/slim) or [Chainguard Apko](https://github.com/chainguard-dev/apko).

## From the idea to an actual tool

With those ideas in mind, we had to transition from [TRLs](https://en.wikipedia.org/wiki/Technology_readiness_level) by implementing it in a tool.
This tool could provide a service, thus the architecture was though as a microservice.

## What's next

- Understand the [Architecture](/docs/chall-manager/design/architecture) of the microservice.
