# Reporting Security Issues

Please report any security issues you discovered in `chall-manager`, `chall-manager-janitor` or the security-by-default recipes of the [SDK](sdk/) to ctfer-io@protonmail.com.

We will assess the risk, plus make a fix available before we create a GitHub issue.

In case the vulnerability is into a dependency, please refer to their security policy directly.

Thank you for your contribution.

## Refering to this repository

To refer to this repository using a CPE v2.3, please use respectively:
- `chall-manager`: `cpe:2.3:a:ctfer-io:chall-manager:*:*:*:*:*:*:*:*`
- `chall-manager-janitor`: `cpe:2.3:a:ctfer-io:chall-manager-janitor:*:*:*:*:*:*:*:*`

Use with the `version` set to the tag you are using.

## Signature and Attestations

For deployment purposes (and especially in the deployment case of Kubernetes), you may want to ensure the integrity of what you run.

The release assets are SLSA 3 and can be verified using [slsa-verifier](https://github.com/slsa-framework/slsa-verifier) using the following.

```bash
slsa-verifier verify-artifact "<path/to/release_artifact>"  \
  --provenance-path "<path/to/release_intoto_attestation>"  \
  --source-uri "github.com/ctfer-io/chall-manager" \
  --source-tag "<tag>"
```

The Docker image is SLSA 3 and can be verified using [slsa-verifier](https://github.com/slsa-framework/slsa-verifier) using the following.

```bash
slsa-verifier slsa-verifier verify-image "ctferio/chall-manager:<tag>@sha256:<digest>" \
    --source-uri "github.com/ctfer-io/chall-manager" \
    --source-tag "<tag>"
```

The same holds for `chall-manager-janitor`.

Alternatives exist, like [Kyverno](https://kyverno.io/) for a Kubernetes-based deployment.

## SBOMs

A SBOM for the whole repository is generated on each release and can be found in the assets of it.
They are signed as SLSA 3 assets. Refer to [Signature and Attestations](#signature-and-attestations) to verify their integrity.

A SBOM is generated for the Docker image in its manifest, and can be inspected using the following.

```bash
docker buildx imagetools inspect "ctferio/chall-manager:<tag>" \
    --format "{{ json .SBOM.SPDX }}"
```
