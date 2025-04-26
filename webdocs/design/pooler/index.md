---
title: Pooler
description: How to preform tremendous performances by pre-provisionning instances.
categories: [Explanations]
tags: [Infrastructure]
weight: 10
---

## Context

Due to [the genericity of the design](/docs/chall-manager/design/genericity/) the abstraction layer imply generic operations. For instance, loading the Pulumi stack might take seconds, increasing the time to handle an API request.
Then, deploying the resources can take several seconds up to several minutes depending on the [scenario](/docs/chall-manager/glossary/#scenario). Finally, writing it all down and serving the result adds more time to the response.

Through this process, there are incompressible times: loading a Pulumi stack, starting the processes, writing on filesystem, etc. The most probable opportunity would be to speed up Pulumi, which is out of our scope and skills.
Nevertheless, what if instead of on-the-fly instances we pre-performed them ?

This is the reason of the `Pooler` feature of Chall-Manager: pre-provision your challenges, make them available in a _pool_, such that when a new instance is requests, it is first picked in the pool if one is available, elseway it is getting deployed.
Experimentation shows the time to claim an instance is under half a second, thus fits the need for [High-Availability](/docs/chall-manager/design/high-availability).

## How it works

Each challenge has its own _pool_ with two attributes:
- `min` defines how many pre-deployed instances are part of the pool ;
- `max` defines a threshold after which it stops pre-provisionning them (could save infrastructure resources, money, ...).

Updates on these attributes resizes the pool according to new desired state, in parallel of the claimed instances stack update.

### Use cases

The following are use cases of the pooler feature of Chall-Manager.

#### Limited resources

Let's say we have a challenge that requires multiple VMs in network. This lab takes too long to deploy, thus would end up frustrating players, degrading the quality of the event.
For this reason, you decide to use the pooler such that there is always some (e.g. 3) labs available for players to instantaneously pick into. Nevertheless, you have 30 teams thus won't require much more.

You set the pooler with:
- `min=3`
- ̀`max=30`

Using these settings, players of your event will have a good feeling about the quality of your event. Nevertheless, reserving resources prior to being used, for time efficiency reasons, imply that your infrastructure will need to be properly dimensioned.

#### Online platform

Let's say you are using Chall-Manager as a backend service for an online cybersecurity training platform. You want to always have available instances for people to train actively rather than clicking and having to wait several minutes -especially your VIPs-.

Based on the usage statistics of previous similar labs, the ads you published around it, you expect an instance to be requested every 10 minutes in average for the upcoming days, with spikes of 1 every minute. Once instance takes (for instance) 6 minutes to dpeloy.
You want to always have 20 instances such that there is also room for people who would like to retry the box from scratch (either they broke it, they want to try an automated solution, speedrun the box, ...).

You set the pooler with:
- `min=20`

These settings expects you have plenty infrastructure capabilities, enough to consider that you won't need a maximum. You actively monitor this to ensure there is no abuse, and if so, you'll set an arbitrary `max` value.
