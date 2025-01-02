### Run Locally

Before running the Controller Manager, ensure that your kubernetes config is connected to the existing Michelangelo
Cluster, or use Sandbox script to create one on your local. The Sandbox script will create a production-like replica of
the Michelangelo Cluster on your machine, and update kubernetes config.

```
sandbox.sh create
```

Run the Controller Manager

```
bazel run //go/cmd/controllermgr
```

Register sample Pipelines

```
kubectl apply -f $WORKSPACE_ROOT/kubernetes/samples/v2beta1_pipeline.yaml
```

Run a sample Pipeline

```
kubectl create -f $WORKSPACE_ROOT/kubernetes/samples/v2beta1_pipeline_run.yaml
```

Don't forget to delete the Sandbox instance, if it was previously created

```
sandbox.sh delete
```

### Build an Image

It's important to build `:image` target with the `--platforms=@io_bazel_rules_go//go/toolchain:linux_amd64` flag as the
binary should be built for Linux since it will run in a Linux container:

```
bazel build //go/cmd/controllermgr:image.tar --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
```

Load an image into the local registry

```
docker load -i $WORKSPACE_ROOT/bazel-bin/go/cmd/controllermgr/image.tar
```

Run the Controller Manager in a container

```
docker run --rm --network=host \
  -e CONFIG_DIR=./go/cmd/controllermgr/config \
  -v $HOME/.kube:/root/.kube \
  bazel/go/cmd/controllermgr:image
```
