---
title: Extensions
description: How to extend capabilities ?
categories: [How-to Guides]
---

How can we extend capabilities ? 
There is no plan to extend chall-manager API with live-mutability of functionalities, plugins, ...
This is motivated by security and functionality supports in downstream services (immutability and determinism are to expect from Chall-Manage).

But as Chall-Manager is designed as a Micro Service, you _only_ have to reuse it !

## Hacking the API

Taking a few steps back, you can abstract the Chall-Manager API to fit your needs:
- the `connection_info` is an **Output** data from the instance to the player.
- the `flag` is an optional **Output** data from the instance to the backend.
Then, if you want to pass additional data, you can use those commmunications buses.

## Case studies

Follows some extension case studies.

### MultiStep & MultiFlags

The original idea comes from the [JointCyberRange](https://jointcyberrange.nl), the following architecture is our proposal to solve the problem.
In the "MultiStep challenge" problem they would like to have Jeopardy-style challenges constructed as a chain of steps, where each step has its own flag. To completly flag the challenge, the player have to get all flags in the proper order. Their target environment is an integration in CTFd as a plugin, and challenges deployed to Kubernetes.

Deploying those instances, isolating them, janitoring if necessary are requirements, thus would have been reimplemented. A clever use of Chall-Manager is to make use of its capabilities.
Our proposal is then to cut the problem in two parts according to the [Separation of Concerns Principle](https://en.wikipedia.org/wiki/Separation_of_concerns):
- a **CTFd plugin** that implement a new challenge type, the multi-step/multi-flag behavior and communicate with Chall-Manager for infrastructure provisionning ;
- **chall-manager** to deploy the instances.

The `connection_info` can be unchanged from its native usage in chall-manager.

The `flag` output could contain a JSON object describing the flags chain and internal requirements.

{{< imgproc multistep-flags-with-chall-manager Fit "800x800" >}}
Our suggested architecture for the JCR MultiStep challenge plugin for CTFd.
{{< /imgproc >}}

Through this architecture, JCR would be able to fit their needs shortly by capitalizing on the chall-manager capabilities, thus extend its goals.
Moreover, it would enable them to provide the community a CTFd plugin that does not only fit Kubernetes, thanks to the [genericity](/docs/chall-manager/design/genericity).
