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

Other features are available in the SDK.

## Flag variation engine

Commonly, each challenge has its own flag. This suffers a big limitation that we can come up to: as each instance is specific to a source, we can define the flag on the fly. But this flag must not be shared with other players or it will enable [shareflag](/docs/chall-manager/design/security#shareflag).

For this reason, we provide the ability to mutate a string (expected to be the flag): for each character, if there is a variant in the ASCII-extended charset, select one of them randomly and based on the [identity](/docs/chall-manager/glossary#identity).

### Variation rules

The variation rules follows, and if a character is not part of it, it is not mutated (each variant has its mutations evenly distributed):
- `a`, `A`, `4`, `@`, `ª`, `À`, `Á`, `Â`, `Ã`, `Ä`, `Å`, `à`, `á`, `â`, `ã`, `ä`, `å`
- `b`, `B`, `8`, `ß`
- `c`, `C`, `(`, `¢`, `©`, `Ç`, `ç`
- `d`, `D`, `Ð`
- `e`, `E`, `€`, `&`, `£`, `È`, `É`, `Ê`, `Ë`, `è`, `é`, `ê`, `ë`, `3`
- `f`, `F`, `ƒ`
- `g`, `G`
- `h`, `H`, `#`
- `i`, `I`, `1`, `!`, `Ì`, `Í`, `Î`, `Ï`, `ì`, `í`, `î`, `ï`
- `j`, `J`
- `k`, `K`
- `l`, `L`
- `m`, `M`
- `n`, `N`, `Ñ`, `ñ`
- `o`, `O`, `0`, `¤`, `°`, `º`, `Ò`, `Ó`, `Ô`, `Õ`, `Ö`, `Ø`, `ø`, `ò`, `ó`, `ô`, `õ`, `ö`, `ð`
- `p`, `P`
- `q`, `Q`
- `r`, `R`, `®`
- `s`, `S`, `5`, `$`, `š`, `Š`, `§`
- `t`, `T`, `7`, `†`
- `u`, `U`, `µ`, `Ù`, `Ú`, `Û`, `Ü`, `ù`, `ú`, `û`, `ü`
- `v`, `V`
- `w`, `W`
- `x`, `X`, `×`
- `y`, `Y`, `Ÿ`, `¥`, `Ý`, `ý`, `ÿ`
- `z`, `Z`, `ž`, `Ž`
- ` `, `-`, `_`, `~`

{{< alert title="Tips & Tricks" color="primary">}}
If you want to use a decorator (e.g. `BREFCTF{...}`), do not put it in the flag to variate. More info [here](/docs/chall-manager/challmaker-guides/flag-variation-engine).
{{< /alert >}}

### Limitations

We are aware that this proposition does not solve all issues: if people share their write-up, they will be able to flag.
This limitation is considered out of our scope, as we don't think the Challenge on Demand solution fits this use case.

Nevertheless, our differentiation strategy can be the basis of a proper solution to the APG-problem (Automatic Program Generation): we are able to write one scenario that will differentiate the instances per source. This could fit the input of an APG-solution.

Moreover, it considers a precise scenario of advanced malicious collaborative sources, where shareflag consider malicious collaborative sources only (more "accessible" by definition).

## Additional configuration

When creating your first scenarios, you have a high coupling between your idea and how it is deployed. But as time goes, you create helper functions that abstracts the complexity and does most of the job for you (e.g. the [`kubernetes.ExposedMonopod`](/docs/chall-manager/challmaker-guides/software-development-kit/#kubernetes-exposedmonopod)).

Despite those improvements, for every challenge that are deployed the same way (for instance, on the NoBrackets 2024, more than 90% of the challenges were deployed by the same scenario with a modified configuration), you have to redo the job multiple times: duplicate, reconfigure, compile, archive, test, destroy, push, ...

Furthermore, if you want to provide fine-grained data to the scenario, you could not. For instance, to edit firewall rules to access a bunch of VMs or a CPS, you may want to provide the scenario the requester IP address. This require on-the-fly configuration to be provided to the scenario when the Instance is created.

To solve both problems, we introduced the **additional configuration** _key=value_ pairs. Both the Challenge and the Instance can provide their configuration pairs to the scenario. They are merged from the Instance's pairs over the Challenge's pairs thus enable _key=value_ pair overwrite if necessary, e.g. to overload a default value.

This open the possibility of creating a small set of scenarios that will be reconfigured on the fly by the challenges (e.g. the previous NoBrackets 2024 example could have run over 2 scenarios for 14 challenges).

## What's next ?

The final step from there is to ensure the quality of our work, with [testing](/docs/chall-manager/design/testing).
