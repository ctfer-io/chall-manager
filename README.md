# Chall-Manager

## Development setup

Once you clonned the repository, run the following commands to make sure you have all the generated files on your local system and up to date.

```bash
make buf
make update-swagger
```

You could also run those before a commit that affects the `*.proto` files to avoid inconsistencies between your local setup and the distant branch.

If you need to run a local etcd instance, you could use the following.

```bash
docker run -v /usr/share/ca-certificates/:/etc/ssl/certs -p 4001:4001 -p 2380:2380 -p 2379:2379 -e ETCD_ROOT_PASSWORD=root bitnami/etcd:3.5.13
```
