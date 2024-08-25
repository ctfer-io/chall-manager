---
title: Chall-Manager
description: Chall-Manager solves the Challenge on Demand problem through a generalization technically based upon Pulumi, and embodies the State-of-the-Art Continuous Deployment practices.
---

Chall-Manager is a MicroService whose goal is to manage challenges and their instances.
Each of those instances are self-operated by the source (either team or user) which rebalance the concerns to empowers them.

It eases the job of Administrators, Operators, Chall-Makers and Players through various features, such as hot infrastructure update, non-vendor-specific API, telemetry and scalability.

Thanks to a generic [Scenario](/docs/chall-manager/glossary#scenario) concept, the Chall-Manager could be operated anywhere (e.g. a Kubernetes cluster, AWS, GCP, microVM, on-premise infrastructures) and run everything (e.g. containers, VMs, IoT, FPGA, custom resources).
