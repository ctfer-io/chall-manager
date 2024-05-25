/*
Package fs wraps filesystem operations to provide a simple and resilient API.

This enable low development and maintainenance effort, while avoiding breaking
changes introduced on the high level of the chall-manager gRPC API (keep it as
simple and readable as possible).

The storage is based on a filesystem, future works could move this to an object
store such a S3-compliant solution.
*/
package fs
