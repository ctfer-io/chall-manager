# Chall-Manager

Chall-Manager is a Kubernetes-native _μService_ that deploys challenge scenario on demand, powered by [Pulumi](https://www.pulumi.com).

<div align="center" width="400px">
    <img src="res/diagram.excalidraw.png">
</div>

For more info on the design of the chall-manager, see the [Design Document](/DESIGN_DOCUMENT.md)

> **Warning**:
>
> A chall-manager will execute API inputs, which could be considered Remote Code Execution.
> This is a voluntary design to fit complex IaC scenarios, which is highly dangerous.
> You should consider shielding your infrastructure that has an instance up and running to
> avoid loosing it to a malicious actor.

> **Warning**:
>
> The only API-contract that is guaranteed through SemVer major versions
> of the `chall-manager` is between the [SDK](#sdk) and the [API](#api), and
> the CLI commands and flags.
> Out of this scope, we suggest you don't rely on internals thus avoid breaking
> changes if we need them.

## Deployment

We recommend running the chall-manager with the gRPC API only.
For ease of development and interoperability, you may want to also run with the gRPC gateway (`--gw`).
For development purposes only, you can run with a REST API swagger (`--gw-swagger`) that you could connect to through `<host>:<gw-port>/swagger/`.

An example for a developer running a local instance would be:
```bash
docker run -p 8080:8080 -p 9090:9090 -d --restart=always pandatix/chall-manager:v0.1.0 --gw --gw-swagger
```

You would be able to connect to the gRPC server at `localhost:8080`, the REST API at `localhost:9090/api/v1/launch` and the swagger at `localhost:9090/swagger/`.

## Development setup

Once you clonned the repository, run the following commands to make sure you have all the generated files on your local system and up to date.

```bash
make buf
make update-swagger
```

You could also run those before a commit that affects the `*.proto` files to avoid inconsistencies between your local setup and the distant branch.
