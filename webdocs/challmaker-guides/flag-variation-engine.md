---
title: Use the flag variation engine
description: Use the flag variation engine to block shareflag, as a native feature of the Chall-Manager SDK.
categories: [How-to Guides]
tags: [Anticheat]
---

Shareflag is considered by some as the worst part of competitions leading to unfair events, while some others consider this a strategy.
We consider this a problem we could solve.

## Context

In "standard" CTFs as we could most see them, it is impossible to solve this problem: if everyone has the same binary to reverse-engineer, how can you differentiate the flag per each team thus avoid shareflag ?

For this, you have to variate the flag for each source. One simple solution is to [use the SDK](#use-the-sdk).

## Use the SDK

The SDK can variate a given input with human-readable equivalent characters in the ASCII-extended charset, making it handleable for CTF platforms (at least we expect it). If one character is out of those ASCII-character, it will be untouched.

To import this part of the SDK, execute the following.

```bash
go get github.com/ctfer-io/chall-manager/sdk
```

Then, in your scenario, you can create a constant that contains the "base flag" (i.e. the unvariated flag).

```go
const flag = "my-supper-flag"
```

Finally, you can export the variated flag.

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

If you want to use decorator around the flag (e.g. `BREFCTF{}`), don't put it in the `flag` constant else it will be variated.
