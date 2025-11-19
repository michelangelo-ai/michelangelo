# Michelangelo-AI

Michelangelo-AI is an open-source platform designed to streamline the development, deployment, and monitoring of machine learning models at scale. It offers a comprehensive suite of tools and services that facilitate the entire machine learning lifecycle, from data management to model serving.

## Features

- **Feature Management**: Efficiently handle large datasets with built-in support for data ingestion, transformation, and storage.
- **Model Training**: Train models using various algorithms, including support for distributed training across multiple nodes.
- **Model Evaluation**: Assess model performance with a range of metrics and visualization tools.
- **Model Deployment**: Seamlessly deploy models to production environments with support for both batch and real-time inference.
- **Monitoring and Logging**: Continuously monitor model performance and log predictions to ensure reliability and accuracy.
- **Proto Extension Framework**: Extend Michelangelo CRDs with organization-specific fields without forking the repository. See [Proto Extensions](PROTO_EXTENSIONS.md) for details.

## Installation

To install Michelangelo-AI, follow [Getting Started](https://github.com/michelangelo-ai/michelangelo/wiki/Getting-Started) guide until GitHub respository is public.

## Usage

Here's a basic example of how to train and deploy a model using Michelangelo-AI:

1. **Data Preparation**: Load and preprocess your dataset.
2. **Model Training**: Use the training module to train your model.
3. **Model Evaluation**: Evaluate the trained model's performance.
4. **Model Deployment**: Deploy the model to the production environment.

For detailed instructions and advanced usage, refer to the [Michelangelo-AI Wiki](https://github.com/michelangelo-ai/michelangelo/wiki).

## Extending Michelangelo

Michelangelo supports extension of its protocol buffer definitions to add organization-specific fields:

```python
# In your organization's repo
load("@michelangelo//bazel/rules/proto:patched_proto.bzl", "patched_proto_library")

patched_proto_library(
    name = "michelangelo_extended",
    base_protos = "@michelangelo//proto/api/v2:v2_proto",
    extension_protos = glob(["extensions/*.proto"]),
    field_prefix = "YOUR_ORG_",
)
```

This allows you to add custom fields like owner IDs, cost centers, or compliance tags without modifying OSS code.

**Learn more**:
- [Proto Extension Framework](PROTO_EXTENSIONS.md) - Overview and quick start
- [Extending Guide](docs/EXTENDING.md) - Comprehensive documentation
- [Examples](examples/extensions/) - Working examples


## Contributing

We welcome contributions to Michelangelo-AI!
If you're interested in contributing, please read our [Contributing Guidelines](https://github.com/michelangelo-ai/michelangelo/wiki/Contributing-Guidelines) to get started.


## License

This project is licensed under [LICENSE](https://github.com/michelangelo-ai/michelangelo/blob/main/LICENSE) before public release.


## Acknowledgments

We would like to thank all the contributors to this project.
