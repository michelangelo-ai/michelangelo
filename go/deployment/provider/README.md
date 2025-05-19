# Spark Job Client

This client is a **demo implementation** of the `Client` interface defined in `interface.go`. It is designed to interact with a specific version of the Spark Operator and its corresponding Kubernetes Custom Resource Definitions (CRDs). The client provides methods to create Spark jobs and retrieve their statuses by leveraging the Spark Operator's API.

## Overview

The `SparkClient` implementation in this package demonstrates how to:

1. **Create Spark Jobs**: Converts the `SparkJob` specification into a `SparkApplication` resource and submits it to the Kubernetes API.
2. **Retrieve Job Status**: Fetches the status of a submitted `SparkApplication` from the Kubernetes API and updates the `SparkJob` status accordingly.

This implementation is tightly coupled with the Spark Operator's CRD definitions and assumes the use of a specific version of the Spark Operator.

## Limitations

- This client is **not generic** and is tailored to work with the Spark Operator's API and CRDs.
- It assumes the presence of the Spark Operator in the Kubernetes cluster and the correct configuration of the CRDs.

## Custom Implementation

If you need to connect to your own control plane or API (e.g., a custom compute team's API), you will need to provide your own implementation of the `Client` interface. This custom implementation should:

1. **Adapt to Your API**: Replace the Spark Operator-specific logic with calls to your compute team's API.
2. **Handle Custom Specifications**: Map the `SparkJob` specification to your API's job definition format.
3. **Retrieve Job Status**: Implement logic to fetch job statuses from your control plane.

## How to Use

1. **Demo Usage**: This client can be used as a reference or for testing purposes in environments where the Spark Operator is deployed.
2. **Custom Implementation**: To connect to your own control plane, implement the `Client` interface defined in `interface.go` and replace the `SparkClient` with your custom implementation.

## Key Files

- `interface.go`: Defines the `Client` interface for managing Spark jobs.
- `client.go`: Contains the `SparkClient` implementation, which interacts with the Spark Operator's API.
- `module.go`: Registers the `SparkClient` with the dependency injection framework.

## Notes

- Ensure that the Spark Operator and its CRDs are installed and configured in your Kubernetes cluster if you plan to use this demo client.
- For production use, adapt the `Client` interface to your specific requirements and implement a custom client to interact with your compute team's API.