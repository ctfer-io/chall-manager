---
title: Entensions
description: How to extend chall-manager capabilities ?
categories: [How-to Guides]
---

How can we extend chall-manager capabilities ? You cannot, in itself: there is no support for live-mutability of functionalities, plugins, nor there will be (immutability is both an operation and security principle, determinism is a requirement).

But as chall-manager is designed as a Micro Service, you _only_ have to reuse it !

## Hacking chall-manager API

Taking a few steps back, you can abstract the chall-manager API to fit your needs:
- the `connection_info` is an **Output** data from the instance to the player.
- the `flag` is an optional **Output** data from the instance to the backend.
Then, if you want to pass additional data, you can use those commmunications buses.

## Case studies

Follows some extension case studies.

### MultiStep & MultiFlags

The original idea comes from the [JointCyberRange](https://jointcyberrange.nl), the following architecture is our proposal to solve the problem.
In the "MultiStep challenge" problem they would like to have Jeopardy-style challenges constructed as a chain of steps, where each step has its own flag. To completly flag the challenge, the player have to get all flags in the proper order. Their target environment is an integration in CTFd as a plugin, and challenges deployed to Kubernetes.

Deploying those instances, isolating them, janitoring if necessary are requirements, thus would have been reimplemented. But chall-manager can deploy scenarios to whatever environment.
Our proposal is then to cut the problem in two parts according to the [Separation of Concerns Principle](https://en.wikipedia.org/wiki/Separation_of_concerns):
- a **CTFd plugin** that implement a new challenge type, and communicate with chall-manager
- **chall-manager** to deploy the instances

The `connection_info` can be unchanged from its native usage in chall-manager.

The `flag` output could contain a JSON object describing the flags chain and internal requirements.

{{< imgproc multistep-flags-with-chall-manager Fit "800x800" >}}
Our suggested architecture for the JCR MultiStep challenge plugin for CTFd.
{{< /imgproc >}}

Through this architecture, JCR would be able to fit their needs shortly by capitalizing on the chall-manager capabilities, thus extend its goals.
Moreover, it would enable them to provide the community a CTFd plugin that does not only fit Kubernetes, thanks to the [genericity](/docs/chall-manager/design/genericity).
