"""Concrete ``ModelRegistryClient`` implementations backed by third-party registries.

Each implementation lives in its own module and imports its backend SDK
lazily, so importing this package never requires any of the optional
registry dependencies to be installed.
"""

from michelangelo.workflow.tasks.pusher.implementations.mlflow_client import (
    MLflowRegistryClient,
)

__all__ = [
    "MLflowRegistryClient",
]
