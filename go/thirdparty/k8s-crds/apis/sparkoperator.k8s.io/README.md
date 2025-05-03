# Spark Operator CRD Definitions in Michelangelo

![Go Report Card](https://goreportcard.com/report/github.com/GoogleCloudPlatform/spark-on-k8s-operator)

The Spark Operator Custom Resource Definitions (CRDs) have been copied directly into Michelangelo's codebase under `go/components/spark/job/client`. This approach avoids potential conflicts arising from managing different versions of the Go modules for the Spark Operator.

In typical production deployments, the control plane managing Spark jobs often resides in a separate cluster, distinct from the Michelangelo control plane environment. This separation of concerns typically results in differing versions of libraries and dependencies across environments.

Including the Spark Operator CRD definitions directly within Michelangelo simplifies dependency management, especially for the Michelangelo sandbox environment, ensuring compatibility and ease of use during development and testing.

**Note:** This implementation is primarily intended for simplified sandbox usage.

> This is not an officially supported Google product.

