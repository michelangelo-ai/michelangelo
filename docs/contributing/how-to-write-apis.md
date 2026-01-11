# Writing APIs
This guide walks through the steps for building and compiling Protocol Buffers (Protos) of ML entities in the Michelangelo using Bazel and Gazelle.

## 1. ML Entities in Proto Files 

Define entities in protobuf in [michelangelo/proto/api/v2](https://github.com/michelangelo-ai/michelangelo/tree/main/proto/api/v2). 

[K8s Controller : Learnings & Best Practices](https://github.com/user-attachments/files/19595646/K8s.Controller_Best.Practices.for.open.source.pdf)

Useful References 
- [kubebuilder](https://book.kubebuilder.io/) 
- [K8s API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Conditions in Kubernetes controllers](https://maelvls.dev/kubernetes-conditions/)
- [Write Kubernetes controllers](https://ahmet.im/blog/controller-pitfalls/)

## 2. Generate gRPC Service Code 
If ML entities are exposed using gRPC services, ensure that the appropriate gRPC code is generated. This usually involves running a go generate command or ensuring Gazelle is configured to handle proto_library and go_proto_library targets for gRPC.

```
# Generate service proto files
tools/grpc-svc-gen.sh [Entity]

# Example for Pipeline 
tools/grpc-svc-gen.sh Pipeline
```

## 3. Generate Proto Files with Gazelle
Before building, make sure all proto files are properly indexed and generated with [Gazelle](https://github.com/bazelbuild/bazel-gazelle), which automatically updates BUILD files for Protocol Buffers:

```
tools/gazelle
```
This will scan your proto directories and update BUILD targets accordingly.

## 4. Build Proto Files
Use Bazel to build the proto targets. This will ensure your .pb.go and other generated files are correctly compiled.
```
bazel build //proto/...
```
To see more detailed error output (helpful for debugging), add the --verbose_failures flag:
```
bazel build //proto/... --verbose_failures
```