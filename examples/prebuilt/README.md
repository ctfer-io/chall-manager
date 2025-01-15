# Prebuilt

The Prebuilt example shows how to make a pre-compiled binary as scenario.

You may want to do it in case:
- you want **high performances** on the challenge creation (avoid Chall-Manager to compile it on its own)
- you want **traceability** on the binary thus provide it to Chall-Manager rather than expecting it to check
- you use a **private dependency** the Chall-Manager has no access to, thus build it locally where it is possible

## Demo

To build the scenario archive you only need to run `./build.sh`.
It will compile the Pulumi Go code and create the zip archive (`scenario.zip`).
