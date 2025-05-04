---
title: Security
description: Explanations on the Security problems that could arise from a chall-manager deployment.
categories: [Explanations]
tags: [Infrastructure, Kubernetes]
weight: 6
---

## RCE-as-a-Service

Through the whole documentation, we often refer to chall-manager as an **RCE-as-a-Service** platform. Indeed it executes [scenarios](/docs/chall-manager/glossary#scenario) on Demand, without authentication nor authorization.

For this reason, we recommend deployments to be deeply burried in the infrastructure, with firewall rules or network policies, encrypted tunnels between the dependent service(s), and anything else applicable.

Under no condition you should launch it exposed to participants and untrusted services.
If not, secrets could be exfiltrated, host platform could be compromised, etc.

> ``With great power comes great responsibility.''
> ~Uncle Ben

## Kubernetes

If you are not using the [recommended architecture](/docs/chall-manager/ops-guides/deployment/#kubernetes), please make sure to not deploy [instances](/docs/chall-manager/glossary#instance) in the same namespace as the chall-manager instances are deployed into. Elseway, players might pivot through the service and use the API for malicious purposes.

Additionally, please make sure the `ServiceAccount` the chall-manager `Pods` use has only its required permissions, and if possible, only on namespaced resources. To build this, you can use `kubectl api-resources –-namespaced=true –o wide`.

## Sharing is caring

As the chall-manager could become costful to deploy and maintain at scale, you may want to share the deployments between multiple plateforms.
Notice the Community Edition does not provide isolation capabilities, so secrets, files, etc. are shared along all [scenarios](/docs/chall-manager/glossary#scenario).
