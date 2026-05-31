"""Model manager library for Michelangelo.

Provides abstractions for the full model lifecycle: interface, schema,
packaging, serialization, and registry.

Public API::

    from michelangelo.lib.model_manager.interface import Model
    from michelangelo.lib.model_manager.schema import (
        DataType, ModelSchema, ModelSchemaItem,
    )
    from michelangelo.lib.model_manager.packager.custom_triton import (
        CustomTritonPackager,
    )
    from michelangelo.lib.model_manager.registry import (
        InMemoryRegistryClient, ModelRegistryClient, RegisteredModel,
    )
"""

# flake8: noqa:F401
from michelangelo.lib.model_manager.registry import (
    InMemoryRegistryClient,
    ModelRegistryClient,
    RegisteredModel,
)
