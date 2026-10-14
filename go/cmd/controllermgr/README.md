# Kubernetes Controller Manager README

## Run Locally

To run the Controller Manager locally, ensure your Kubernetes configuration is connected to the existing Michelangelo AI Cluster. Alternatively, you can use the sandbox script to create a production-like replica of the Michelangelo AI Cluster locally, which will also update your Kubernetes configuration.

1. **Create a sandbox cluster:**
```bash
ma sandbox create
```

2. **Start the Controller Manager:**
```bash
bazel run //go/cmd/controllermgr
```

## Build and Run in a Container

The release images are built from [`docker/service.Dockerfile`](../../../docker/service.Dockerfile), which copies a prebuilt static binary and the service config directory into a distroless base image.

1. **Build the static Linux binary** (from the repository root; the binary is written to the repo root):
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o controllermgr ./go/cmd/controllermgr
```
Adjust `GOARCH` to `arm64` on Apple Silicon.

2. **Build the image** (from the repository root):
```bash
docker build -f docker/service.Dockerfile \
  --build-arg BINARY_PATH=controllermgr \
  --build-arg CONFIG_PATH=go/cmd/controllermgr/config \
  -t michelangelo-controllermgr:local .
```

3. **Run the Controller Manager in a container:**
```bash
docker run --rm --network=host \
  -e CONFIG_DIR=/config \
  -v $HOME/.kube:/root/.kube \
  michelangelo-controllermgr:local
```
