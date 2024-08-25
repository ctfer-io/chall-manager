---
title: Software Development Kit
description: Learn how we designed a SDK to ease the use of chall-manager for non-DevOps people.
categories: [Explanations]
tags: [Infrastructure, Kubernetes, AWS, GCP]
weight: 9
---

A first comment on chall-manager was that it required [ChallMaker](/docs/chall-manager/glossary#challmaker) and [Ops](/docs/chall-manager/glossary#) to be DevOps. Indeed, if we expect people to be providers' experts to deploy a challenge, when there expertise is on a cybersecurity aspect... well, it is incoherent.

To avoid this, we took a few steps back and asked ourselves: for a beginner, what are the deployment practices that could arise form the use of chall-manager ?

A naive approach was to consider the deployment of a single Docker container in a Cloud provider (Kubernetes, GCP, AWS, etc.).
For this reason, we implemented the minimal requirements to effectively deploy a Docker container in a Kubernetes cluster, exposed through an Ingress or a NodePort. The results were hundreds-line-long, so confirmed we cannot expect non-professionnals to do it.

Based on this experiment, we decided to reuse this Pulumi scenario to build a Software Development Kit to empower the [ChallMaker](/docs/chall-manager/glossary#challmaker). The references architectures contained in the SDK are available [here](/docs/chall-manager/challmaker-guides/software-development-kit).
The rule of thumb with them is to infer the most possible things, to have a mimimum configuration for the end user.

## What's next ?

The final step from there is to ensure the quality of our work, with [testing](/docs/chall-manager/design/testing).
