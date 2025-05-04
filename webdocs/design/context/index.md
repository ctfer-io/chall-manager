---
title: Context
description: What is the need for Challenge on Demand ?
weight: 1
---

A Capture The Flag (CTF) is an event that brings people the compete, to challenge themselves and others on domain-specific problems.
These events could have various objectives such as learning, competing with cashprizes, etc.
They could be held physically, virtually or in a hybrid mode.

CTFs are largely adopted in the cybersecurity community, taking place [all over the world on a daily basis](https://ctftime.org).
In this community, plenty expertises are represented: web, forensic, Active Directory, cryptography, game hacking, telecommunications, steganography, mobile, OSINT, reverse engineering, programming, and so on.

In general, a challenge is composed of a name, has a description, a set of hints, files and other data, shared for all. On top of those, the competition runs over points displayed on scoreboards: this is how people keep getting entertained throughout a continuous and hours-long rush.
Most of the challenges find sufficient solutions to their needs with those functionalities, perhaps some does not...

If we consider other aspects of cybersecurity -as infrastructures, telecommunications, satellites, web3 and others- those solutions are not sufficient.
They need specific deployments strategies, and are costfull to deploy even only once.

Nevertheless, with the emergence of the Infrastructure as Code paradigm, we think of infrastructures as reusable components, composing with pieces like for a puzzle. Technologies appeared to embrace this new paradigm, and where used by the cybersecurity community to build CTF infrastructures (e.g. [BreizhCTF](https://github.com/BreizhCTF/breizhctf-2023/tree/main/infra), [CloudVillage](https://github.com/cloud-village/ctfd-infra)).
Parts of these infrastructures are dedicated to some challenges in which players compete in parallel.

Nonetheless, one _de facto_ problem lays in sharing infrastructure. Indeed, if you are running a CTF to select the top players for a larger event, how would you be able to determine who performed better ? How do you assure their success is due to their sole effort and not a side effect of someone else work ? How to enable large-impact techniques ?
In the same way, from the player perspective, you could end up incapable of solving a challenge simply because someone broke it and the Ops were not aware of this. Side effects could make the efforts worthless.

This is the reason for Challenge on Demand: **each participant** (either a user or a team, which we refer to as a _source_) has **its own instance**. There is **no side effects possible** thanks to isolation of these instances. It also simplifies the way ChallMaker conceive challenges, as they do not require to endorse parallel sandboxing principles but can shift their focus toward the quality of the challenge itself.

## What's next ?

Read [The Need](/docs/chall-manager/design/the-need) to clarify the necessity of Challenge on Demand.
