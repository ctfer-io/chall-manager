---
title: Use the flag variation engine
description: Use the flag variation engine to block shareflag, as a native feature of the Chall-Manager SDK.
categories: [How-to Guides]
tags: [Anticheat]
---

Shareflag is widely frowned upon by participants, as it undermines the spirit of fair play and learning.
It not only skews the scoreboard but also devalues the hard work of those who solve challenges independently. This behavior creates frustration among honest teams and can diminish the overall experience, turning the event into a disheartening one.

Nevertheless, Chall-Manager enables you to **solve shareflag**.

## Context

In common CTFs, a challenge has a flag that must be found by players in order to claim the points. But it implies that everyone has been provided the same content, e.g. the same binary to reverse-engineer.
With such approach, how can you differentiate the flag per each team thus detect -if not avoid- shareflag ?

A perfect solution would be to have a flag for each source. One simple solution is to [use the SDK](#use-the-sdk).

## Use the SDK

The SDK can **variate** a given input with human-readable equivalent characters in the ASCII-extended charset, making it handleable for CTF platforms (or at least we expect so). If one character is out of those ASCII-character, it will remain unchanged.

In your scenario, you can create a constant that contains the original flag.

```go
const flag = "my-supper-flag"
```

Then, you can simply pass it to the SDK to variate it. It will use the identity of the instance as the PRNG seed, thus you should avoid passing it to the players.

If you want to use decorator around the flag (e.g. `BREFCTF{}`), don't put it in the `flag` constant else it will be variated.

A complete example follows.

{{< tabpane code=true >}}
{{< tab header="SDK" lang="go" >}}
package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/ctfer-io/chall-manager/sdk"
)

const flag = "my-supper-flag"

func main() {
	sdk.Run(func(req *sdk.Request, resp *sdk.Response, opts ...pulumi.ResourceOption) error {
		// ...

		resp.ConnectionInfo = pulumi.String("...").ToStringOutput()
		resp.Flag = pulumi.Sprintf("BREFCTF{%s}", sdk.VariateFlag(req.Config.Identity, flag))
		return nil
	})
}
{{< /tab >}}
{{< tab header="No SDK" lang="go" >}}
package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

    "github.com/ctfer-io/chall-manager/sdk"
)

const flag = "my-supper-flag"

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// 1. Load config
		cfg := config.New(ctx, "no-sdk")
		config := map[string]string{
			"identity": cfg.Get("identity"),
		}

		// 2. Create resources
		// ...

		// 3. Export outputs
		ctx.Export("connection_info", pulumi.String("..."))
        ctx.Export("flag", pulumi.Sprintf("BREFCTF{%s}", sdk.VariateFlag(config["identity"], flag)))
		return nil
	})
}
{{< /tab >}}
{{< /tabpane >}}
