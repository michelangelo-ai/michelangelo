# Kubernetes APIServer README

## Build and Run in a Container
To build the `:image`  target, use the following command with the specified platform flag for Linux containers:

```bash
bazel build //go/cmd/apiserver:image.tar --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
```

Load the Image into Docker
1. **Load the generated image into your local Docker registry:**
```bash
docker load -i $WORKSPACE_ROOT/bazel-bin/go/cmd/apiserver/image.tar
```

Run the Controller Manager in a Container

2. **Load the generated image into your local Docker registry:**
```bash
docker run --rm --network=host \
  -e CONFIG_DIR=./go/cmd/apiserver/config \
  -v $HOME/.kube:/root/.kube \
  bazel/go/cmd/apiserver:image
```

By following these instructions, you can effectively run, build, and deploy the Kubernetes Controller Manager locally or in a containerized environment.
