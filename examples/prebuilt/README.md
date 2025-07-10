# Prebuilt

The Prebuilt example shows how to make a pre-compiled binary as scenario.

You may want to do it in case:
- you want **high performances** on the challenge creation (avoid Chall-Manager to compile it on its own)
- you want **traceability** on the binary thus provide it to Chall-Manager rather than expecting it to check
- you use a **private dependency** the Chall-Manager has no access to, thus build it locally where it is possible

## Demo

Requirements:
- [ORAS CLI](https://oras.land/docs/installation#release-artifacts) ;
- a Docker registry (ex: `docker run -d -p 5000:5000 --name registry registry:2 && export REGISTRY="localhost:5000/"`).

To build and push the scenario you only need to run `./build.sh`.
It will compile the Pulumi Go program, add it and the `Pulumi.yaml` file into an OCI artefact, then push it in a registry.
