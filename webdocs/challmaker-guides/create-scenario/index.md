---
title: Create a Scenario
description: Create a Chall-Manager Scenario from scratch.
weight: 1
categories: [How-to Guides]
tags: [SDK, AWS]
---

You are a [ChallMaker](/docs/chall-manager/glossary#challmaker) or only curious ?
You want to understand how the chall-manager can spin up challenge instances on demand ?
You are at the best place for it then.

This tutorial will be split up in three parts:
- [Design your Pulumi factory](#design-your-pulumi-factory)
- [Make it ready for chall-manager](#make-it-ready-for-chall-manager)
- [Use the SDK](#use-the-sdk)

## Design your Pulumi factory

We call a "Pulumi factory" a golang code or binary that fits the chall-manager [scenario](/docs/chall-manager/glossary#scenario) API.
For details on this API, refer to the [SDK documentation](/docs/chall-manager/explanations/software-development-kit#API).

The requirements are:
- have [go](https://go.dev/doc/install) installed.
- have [pulumi](https://www.pulumi.com/docs/install/) installed.

Create a directory and start working in it.

```bash
mkdir my-challenge
cd $_

go mod init my-challenge
```

First of all, you'll configure your Pulumi factory.
The example below constitutes the minimal requirements, but you can add [more configuration](https://www.pulumi.com/docs/languages-sdks/yaml/yaml-language-reference/) if necessary.

{{< card code=true header="`Pulumi.yaml`" lang="yaml" >}}
name: my-challenge
runtime: go
description: Some description that enable others understand my challenge scenario.
{{< /card >}}

Then create your entrypoint base.

{{< card code=true header="`main.go`" lang="go" >}}
package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
        // Scenario will go there

		return nil
	})
}
{{< /card >}}

You will need to add `github.com/pulumi/pulumi/sdk/v3/go` to your dependencies: execute `go mod tidy`.

Starting from here, you can get configurations, add your resources and use various [providers](https://www.pulumi.com/registry/).

For this tutorial, we will create a challenge consuming the [identity](/docs/chall-manager/glossary#identity) from the configuration and create an Amazon S3 Bucket. At the end, we will export the `connection_info` to match the [SDK API](/docs/chall-manager/explanations/software-development-kit#API).

{{< card code=true header="`main.go`" lang="go" >}}
package main

import (
    "github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
        // 1. Load config
		cfg := config.New(ctx, "my-challenge")
		config := map[string]string{
			"identity": cfg.Get("identity"),
		}

        // 2. Create resources
        _, err := s3.NewBucketV2(ctx, "example", &s3.BucketV2Args{
			Bucket: pulumi.String(config["identity"]),
			Tags: pulumi.StringMap{
				"Name":     pulumi.String("My Challenge Bucket"),
				"Identity": pulumi.String(config["identity"]),
			},
		})
		if err != nil {
			return err
		}

        // 3. Export outputs
        // This is a mockup connection info, please provide something meaningfull and executable
		ctx.Export("connection_info", pulumi.String("..."))
		return nil
	})
}
{{< /card >}}

Don't forget to run `go mod tidy` to add the required Go modules. Additionally, make sure to configure the `chall-manager` pods to get access to your [AWS configuration](https://www.pulumi.com/registry/packages/aws/installation-configuration/) through environment variables, and add a Provider configuration in your code if necessary.

{{< alert title="Tips & Tricks" color="primary" >}}
You can compile your code to make the challenge creation/update faster, but chall-manager will automatically do it anyway to enhance performances (avoid re-downloading Go modules and Pulumi providers, and compile the scenario).
Such build could be performed through `CGO_ENABLED=0 go build -o main path/to/main.go`.

Add the following configuration in your `Pulumi.yaml` file to consume it, and set the binary path accordingly to the filesystem.
```yaml
runtime:
  name: go
  options:
   binary: ./main
```
{{< /alert >}}

You can test it using the Pulumi CLI with for instance the following.
```bash
pulumi stack init # answer the questions
pulumi up         # preview and deploy
```

## Make it ready for chall-manager

Now that your scenario is designed and coded accordingly to your artistic direction, you have to prepare it for the chall-manager to receive it.
Make sure to remove all unnecessary files, and zip the directory it is contained within.

```bash
cd ..
zip -r my-challenge.zip ./my-challenge/*
```

And you're done. Yes, it was that easy :)

But it could be even more [using the SDK](/docs/chall-manager/challmaker-guides/software-development-kit) !

{{< alert title="Tips & Tricks" color="primary" >}}
You don't need to archive all files.

If you don't pre-compiled your [scenario](/docs/chall-manager/glossary#scenario), you need to archive all source files.

If you prebuilt the [scenario](/docs/chall-manager/glossary#scenario), you'll only need to pack the `main` binary and `Pulumi.yaml` file.
{{< /alert >}}
