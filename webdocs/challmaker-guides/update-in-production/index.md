---
title: Update in production
description: >
    How to update a challenge [scenario](/docs/chall-maanger/glossary#scenario) once it is in production (instances are deployed) ?
categories: [Explanations]
tags: [The Update Framework]
resources:
- src: "**.png"
---

So you have a challenge that made its way to production, but it **contains a bug** or an **unexpected solve** ?
Yes, we understand your pain: you would like to patch this but expect services interruption... It is not a problem anymore !

{{< imgproc workflow Fit "800x800" >}}
A common worklow of a challenge fix happening in production.
{{< /imgproc >}}

We adopted the reflexions of [The Update Framework](https://theupdateframework.io/) to provide infrastructure update mecanisms with different properties.

## What to do

You will have to create a new [scenario](/docs/chall-manager/glossary#scenario), of course.
Then, you will have to update the challenge configuration to provide this new scenario and an update strategy. If no strategy is specified, it defaults to `update-in-place`.

Chall-Manager will temporarily block operations on this challenge, and update all existing instances.
This makes the process predictible and reproductible, thus you can test in a pre-production environment before production (and we recommend you so). It also avoids human errors during fix, and lower the burden at scale.

| Update Strategy | Require Robustness¹ | Time efficiency | Cost efficiency | Availability | TL;DR; |
|---|:---:|:---:|:---:|:---:|---|
| Update in place | ✅ | ✅ | ✅ | ✅ | Efficient in time & cost ; require high maturity |
| Blue-Green      | ❌ | ✅ | ❌ | ✅ | Efficient in time ; costfull |
| Recreate        | ❌ | ❌ | ✅ | ❌ | Efficient in cost ; time consuming |

¹ Robustness of both the provider and resources updates. Robustness is the capability of a scenario to be finely updated without complete re-creation.

More information on how they work internally is available in the [design documentation](/docs/chall-manager/design/hot-update).
