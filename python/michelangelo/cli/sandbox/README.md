Sandbox is a lightweight version of the Michelangelo cluster, designed specifically for development and testing. It also serves as an excellent tool for users to quickly explore the platform and familiarize themselves with its interface.

> **Note:** The Sandbox deployment is intended for development and testing purposes only and is not suitable for production environments.
> For guidance on creating a production-ready Michelangelo deployment, please refer to the Deployment Guide.

## User Guide

### Prerequisites

**Required Software**

Please install the following software before proceeding:

- [Docker](https://docs.docker.com/get-started/get-docker)
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [k3d](https://k3d.io)

**GitHub Personal Access Token**

Michelangelo is not publicly available yet, so we keep Michelangelo's Docker containers in the private GitHub Container Registry, which requires a GitHub personal access token (classic) for authentication.

To enable authentication for the sandbox, please create a GitHub personal access token (classic) with the "read:packages" scope and save it to the `CR_PAT` environment variable. For example, you can add the following line to your shell configuration file (such as `.bashrc` or `.zshrc`, depending on the shell you use):

```bash
export CR_PAT=your_token_...
```

For a more detailed guide, please refer to [Authenticating with a Personal Access Token (classic)](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic).

> **Be aware:** The `CR_PAT` environment variable is required while Michelangelo is NOT publicly accessible. Once Michelangelo becomes public, this token will no longer be necessary, and this section will be removed.

TODO: andrii: remove this section after the public release of Michelangelo

### Install Michelangelo CLI

```bash
pip install michelangelo
ma sandbox --help
```

## Workflow Engine Options

You can choose the workflow engine when creating a Michelangelo sandbox:

- To create a sandbox using **Temporal**, use:

```bash
ma sandbox --workflow temporal
```

- To create a sandbox using **Cadence**, use either of the following commands:

```bash
ma sandbox
# or explicitly
ma sandbox --workflow cadence
```

For detailed instructions and additional setup options, please follow the [Temporal Development Environment Guide](https://learn.temporal.io/getting_started/typescript/dev_environment/).

## Monitoring and Metrics

The sandbox includes monitoring capabilities with Grafana and Prometheus to visualize CRD schema validation metrics from the controller manager.

### Setting up Monitoring

1. **Deploy Prometheus and Grafana:**
   ```bash
   kubectl apply -f resources/prometheus.yaml
   kubectl apply -f resources/grafana.yaml
   ```

2. **Wait for deployments to be ready:**
   ```bash
   kubectl wait --for=condition=available deployment/prometheus --timeout=60s
   kubectl wait --for=condition=available deployment/grafana --timeout=60s
   ```

3. **Set up port forwarding:**
   ```bash
   # Forward Grafana (runs in background)
   kubectl port-forward svc/grafana 3000:3000 &
   
   # Forward Prometheus (runs in background)  
   kubectl port-forward svc/prometheus 9090:9090 &
   ```

4. **Access the monitoring dashboards:**
   - **Grafana**: http://localhost:3000 (admin/admin)
   - **Prometheus**: http://localhost:9090

### Available Metrics

The controller manager exposes the following CRD unmarshal metrics:

- `crd_unmarshal_success_resource_type_Pipeline_field_type_spec` - Successful Pipeline spec unmarshals
- `crd_unmarshal_errors_resource_type_Pipeline_field_type_spec_error_type_unmarshal_error` - Pipeline spec unmarshal errors

### Creating Custom Dashboards

In Grafana, you can create dashboards using these Prometheus queries:

- **Error rate**: `rate(crd_unmarshal_errors_resource_type_Pipeline_field_type_spec_error_type_unmarshal_error[5m])`
- **Success count**: `crd_unmarshal_success_resource_type_Pipeline_field_type_spec`
- **Total errors**: `sum(crd_unmarshal_errors_resource_type_Pipeline_field_type_spec_error_type_unmarshal_error)`

### Architecture

The monitoring setup consists of:

1. **Controller Manager** - Exposes metrics at `:8090/metrics` in Prometheus format
2. **Prometheus** - Scrapes metrics from the controller manager every 5 seconds
3. **Grafana** - Queries Prometheus for visualization and dashboards

The controller manager runs on the host (outside k3d) while Prometheus and Grafana run inside the k3d cluster. Network connectivity is established using the host gateway IP (`192.168.65.254:8090`).

