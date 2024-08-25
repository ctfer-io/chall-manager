---
title: Glossary
description: The concepts used or introduced by the Chall-Manager.
weight: 999
resources:
- src: "**.png"
---

## Challenge on Demand

The capacity of a CTF platform to empower a [source](#source) to deploy its own challenges autonomously.

## Scenario

It is the refinement of an artistic direction for a CTF.

In the case of Chall-Manager, it could be compared as the recipe of deployment for a given challenge.
Technically, the scenario is a Pulumi entrypoint written in Go that conforms to the SDK.
When launched, it deploys the [source](#source)'s infrastructure and return data such as the connection information or an instance-specific flag.

## Source

Either a team or user at the origin of a request.
For abstraction purposes, we consider it being the same under the use of the "source" term.

## Identity

An identity tie a challenge, a [source](#source) and an [instance](#instance) request together. This last one is random (crypto) thus can't be guessed.
It enable the chall-manager to strictly identify resources as part of separate [instances](#instance) running at the same spot, and provide the scenario a reproductible random seed in case of update (idempotence is not guaranteed through the challenge lifecycle).

{{< imgproc identity Fit "800x800" >}}
Identity production process.
{{< /imgproc >}}

## Instance

An instance is the product of a scenario, once launched with an identity.

## Player

A player is a CTF participant who is going to manipulate instances of challenges throughout the lifetime of the event.

## ChallMaker

The designer of the challenge, often with a security expert profile on the category (s)he contributes to.
This is an essential role for a CTF event, as without them, the CTF would simply not exist !

Notice it is the **responsibility of the ChallMaker** to make its challenge **playable**, not the [Ops](#ops).
If you can't make your challenge run into pre-prod/prod, you can't blame the Ops.

(S)He collaborates with plenty profiles:
- other ChallMakers to debate its ideas and assess the difficulty.
- [Ops](#ops) to make sure its challenges can reach production smoothly.
- [Admins](#administrator) to discuss the technical feasibility of its challenges, for instance if it requires FPGAs, online platforms as GCP or AWS, etc. or report on the status of the CTF.
- an artistic direction, graphical designer, etc. to assist on the coherence of the challenge in the whole artistic process.

## Ops

The Operator of the event who ensure the infrastructure is up and running, everything runs untroubled thus players can compete.
They do not need to be security experts, but might probably be due to the community a CTF brings.

They are the rulers of the infrastructure, its architecture and its incidents. ChallMakers have both fear and admiration on them, as they enable playing complex scenarios but are one click away of destructing everything.

(S)He collaborates with various profiles:
- other Ops as a rubber ducky, a mental support during an outage or simply to work in group.
- [ChallMakers](#challmaker) to assist writing the [scenarios](#scenario) in case of a difficulty or a specific infrastructure architecture or requirement.
- [Admins](#administrator) to report on the current lifecycle of the infrastructures, the incidents, or provide ideas for evolutions such as a partnership.
- a technical leader to centralize the reflexions on architectures and means to enable the artistic direction achieving their goals.

## Administrator

The Administrator is the showcase of the event. (S)He takes responsibility and decisions during the creation process of the event, make sure to synchronize teams throughout the development of the artistic and technical ideas, and manage partnerships if necessary. They are the managers through the whole event, before and after, not only during the CTF.

(S)He basically collaborates with everyone, which is a double-edged sword: you take the gratification of the whole effort, but have no time to rest.
