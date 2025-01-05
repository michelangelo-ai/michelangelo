# Kubernetes Controller Manager README

## Run Locally

To run the Controller Manager locally, ensure your Kubernetes configuration is connected to the existing Michelangelo Cluster. Alternatively, you can use the sandbox script to create a production-like replica of the Michelangelo Cluster locally, which will also update your Kubernetes configuration.

1. **Create a sandbox cluster:**
```bash
sandbox.sh create
```

2. **Start the Controller Manager:**
```bash
bazel run //go/cmd/controllermgr
```

3. **Register sample pipelines:**
```bash
kubectl apply -f $WORKSPACE_ROOT/kubernetes/samples/v2beta1_pipeline.yaml
```

4. **Run a sample pipeline:**
```bash
kubectl create -f $WORKSPACE_ROOT/kubernetes/samples/v2beta1_pipeline_run.yaml
```

5. **Delete the sandbox instance (if previously created):**
```bash
sandbox.sh delete
```

## Build and Run in a Container
To build the `:image`  target, use the following command with the specified platform flag for Linux containers:

```bash
bazel build //go/cmd/controllermgr:image.tar --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
```

Load the Image into Docker
1. **Load the generated image into your local Docker registry:**
```bash
docker load -i $WORKSPACE_ROOT/bazel-bin/go/cmd/controllermgr/image.tar
```

Run the Controller Manager in a Container

2. **Load the generated image into your local Docker registry:**
```bash
docker run --rm --network=host \
  -e CONFIG_DIR=./go/cmd/controllermgr/config \
  -v $HOME/.kube:/root/.kube \
  bazel/go/cmd/controllermgr:image
```

By following these instructions, you can effectively run, build, and deploy the Kubernetes Controller Manager locally or in a containerized environment.
