---
title: Genericity
description: What is the layer of genericity ?
categories: [Explanations]
weight: 3
---

Providing a **generic** approach is fundamental for Chall-Manager. With infrastructure management, we want to apply functionalities whatever the technologies are in use. In the core of the approach, we define the _genericity layer_ as this API, capable of handling whatever technology is in use.

While trying to experimentally conceive a Proof-of-Concept for this layer, we first wanted to create _lambdas_. We quickly ended up unsatisfied by the approach, as it required us to build a lot of tools, practices, maintain a large code base, and would have most likely ended up with a vendor-lock-in solution... We wanted to find an already-existing technology that would give us the basic capabilities.

After days of thinking, we realised one of our tool was exactly doing it: [Victor](https://github.com/ctfer-io/victor). Indeed, Victor is able to deploy any Pulumi program, used for Continuous Delivery. This imply that the solution was already before our eyes: a Pulumi program.
It can run anything as long as it has a provider for it, and at the time of writing there is a large ecosystem of nearly 300 providers. Then Pulumi provides us the way to manage instances programmatically.

This Pulumi program technically forms the **genericity layer**, as easy as this.

## Pulumi as the solution

To go from theory to practice, we had to make choices.
One of the problem with a large genericity is it being... large, actually.

If you consider all ecosystems covered by Pulumi, to cover them you'll require all runtimes installed on the host machine.
For instance, the [Pulumi Docker image](https://hub.docker.com/r/pulumi/pulumi) is around 1.5 GB. This imply that a generic solution covering all ecosystems would be around 2 GB of memory.

Moreover, the enhancements you can propose in a language would have to be re-implemented similarly in every language, or transpiled. As [transpilation](https://en.wikipedia.org/wiki/Source-to-source_compiler) is a heavy task, either manual or automatic but with a high error rate, it is not suitable for production.

Our choice was to **focus on one language first** (Golang), and later permit transpilation in other languages if technically automatable with a high success rate.
With this choice, we would only have to deal with the [Pulumi Go Docker image](https://hub.docker.com/r/pulumi/pulumi-go), around 200 MB (a 7.5 reduction factor). It could be even more reduced using minified images, using [the Slim Toolkit](https://github.com/slimtoolkit/slim) or [Chainguard Apko](https://github.com/chainguard-dev/apko).
Moreover, deep optimisations could be performed thanks to the compilation of the Go codebase when the challenge is created, if not already performed.

## From the idea to an actual tool

From Victor, we had to transition from [TRLs](https://en.wikipedia.org/wiki/Technology_readiness_level) by implementing it as a service.
This service would focus on deploying challenge instances on demand, thus was built as a Micro Service.

Thanks to Micro Service Architectures (MSA), integrating in platforms or extending it would be possible.
We can then imagine plenty other challenges kind that would require [Challenge on Demand](/docs/chall-manager/glossary#challenge-on-demand):
1. King of the Hill
2. Attack & Defense (1 vs 1, 1 vs n, 1 vs bot)
3. [MultiSteps & MultiFlags](/docs/chall-manager/dev-guides/extensions) (for Jeopardy)

We already plan creating 1 and 2.

## What's next

- Understand the [Architecture](/docs/chall-manager/design/architecture) of the Micro Service.
