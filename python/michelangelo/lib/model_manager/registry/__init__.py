"""Model registry client interface for the model manager."""

# flake8: noqa:F401
from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
from michelangelo.lib.model_manager.registry.client import (
    InMemoryRegistryClient,
    ModelRegistryClient,
    RegisteredModel,
)
from michelangelo.lib.model_manager.registry.schema.api import APIRegistryConfig
