"""Model registry client interface for the model manager.

Production configuration
------------------------

- :class:`APIRegistryClient` requires a running Michelangelo ``ModelService``
  gRPC endpoint. Set ``insecure=False`` for any TLS-protected production
  endpoint (``insecure=True`` is the default for local sandbox use only).
- Always call :meth:`APIRegistryClient.close` or use the client as a context
  manager (``with APIRegistryClient(...) as client:``) to release gRPC channel
  resources when done.
- :class:`InMemoryRegistryClient` is for unit tests and quick-start examples
  only — all state is lost when the process exits.
"""

# flake8: noqa:F401
from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
from michelangelo.lib.model_manager.registry.client import (
    InMemoryRegistryClient,
    ModelRegistryClient,
    RegisteredModel,
)
