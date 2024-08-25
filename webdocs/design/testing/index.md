---
title: Testing
description: >
    Building something cool is a thing, assuring its quality is another.
    Learn how we dealt with this specific service Integration, Verification and Validation, especially through [Romeo](/docs/romeo).
categories: [Explanations]
tags: [Testing]
weight: 10
math: true
---

Testing a solution can contain many phases. As a good practice, those tests strategies should be reproducible, explainable and run frequently.

In the case of CTFer.io, we adopt good practices and challenge them to improve.
Given this context, the `chall-manager` has a specificity: it is _only_ a wrapper around the [Pulumi automation API](https://github.com/pulumi/pulumi/tree/master/sdk/go/auto). There are nearly no operations related to something else than filesystem or gRPC calls.

To test code, a common approach is to write unitary tests. Those will ensure the compliance and proper work of functionalities. Running them frequently (automated in CI and through the development lifecycle) ensure no regressions.
By measuring code coverage, it gives us information on the actual code not prone to regressions.
But unitary tests require no interactions with the filesystem, syscalls, external services. The problem with the `chall-manager` is that it is composed only of those. Using unit testing we would only be able to cover \\(<10 \\%\\) of the code, not a sufficient level to be confident in its quality and evolutions.

The other approaches would be to create functional and integration tests. Thanks to [Go 1.20 compiler feature to compile binaries with the `-cover` flag](https://tip.golang.org/doc/go1.20#cover) to provide coverage data on run, we can trigger the tests and measure their impact on the covered code.
To write functional and integration tests, we need to deploy the `chall-manager`. As gophers, we want this deployment to be in Go, the tests also. We could capitalize on competencies in one language and make sure to maintain capabilities throughout the maintainers and contributors.

Fortunately, we could:
- use Go as the development language
- compile the binary with the `-cover` flag
- write unit tests in Golang. They are all prefixed by `Test_U_`.
- write functional and integration tests in Golang, using the [Pulumi integration API](https://www.pulumi.com/docs/using-pulumi/testing/integration/).  They are all prefixed by `Test_F_` or `Test_I_`.
- measure code coverage from unit, functional and integration tests with [`Romeo`](https://github.com/ctfer-io/romeo).

Thanks to this process of testing, we are able to achieve \\(>85 \\%\\) code coverage thus be confident in our work through time.
Moreover, as we systematically run those tests on Pull Requests, we can ensure [dependabot](https://github.com/dependabot) updates compatibility or contributions quality.

Finally, we test all the [examples](https://github.com/ctfer-io/chall-manager/tree/main/examples) to ensure documentation validity.
