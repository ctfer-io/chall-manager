# No-SDK

The No-SDK example provide you a Chall-Manager-SDK-free code for you to support the main features of a scenario.

You may want to manipulate further your stack or do pre-flight checks, thus do this.
In most cases, you should **not** do it, especially if you do not know what you are doing.

## Demo

Requirements:
- [ORAS CLI](https://oras.land/docs/installation#release-artifacts) ;
- [yq](https://github.com/mikefarah/yq) ;
- a Docker registry (ex: `docker run -d -p 5000:5000 --name registry registry:2 && export REGISTRY="localhost:5000/"`).

To build and push the scenario you only need to run `./build.sh`.
It will compile the Pulumi Go program, add it and the `Pulumi.yaml` file into an OCI artefact, then push it in a registry.
