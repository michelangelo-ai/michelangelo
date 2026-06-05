"""Michelangelo pusher — push ML artifacts to storage and registry destinations.

Public API
----------

**Dispatch:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import push

**Configuration:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        PusherConfig,
        PusherPluginConfig,
        ModelPluginConfig,
        DatasetPluginConfig,
        EvalReportPluginConfig,
    )

**Results:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import PusherResult

**Plugins:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        ModelPusherPlugin,
        DatasetPusherPlugin,
        EvalReportPusherPlugin,
    )

**Registry (for plugin extension):**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import default_registry, PluginRegistry

**Exceptions:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        PusherError,
        ArtifactNotFoundError,
        PusherPluginError,
        ConfigurationError,
    )
"""

from michelangelo.workflow.schema.pusher import (
    DatasetPluginConfig,
    EvalReportPluginConfig,
    ModelPluginConfig,
    PusherConfig,
    PusherPluginConfig,
)
from michelangelo.workflow.tasks.pusher.exceptions import (
    ArtifactNotFoundError,
    ConfigurationError,
    PusherError,
    PusherPluginError,
)
from michelangelo.workflow.tasks.pusher.plugins import (
    DatasetPusherPlugin,
    EvalReportPusherPlugin,
    ModelPusherPlugin,
    ModelPushResult,
    PartialRegistrationError,
    RegistrationResult,
)
from michelangelo.workflow.tasks.pusher.pusher import push
from michelangelo.workflow.tasks.pusher.registry import PluginRegistry, default_registry
from michelangelo.workflow.variables.types import PusherResult

__all__ = [
    "ArtifactNotFoundError",
    "ConfigurationError",
    "DatasetPluginConfig",
    "DatasetPusherPlugin",
    "EvalReportPluginConfig",
    "EvalReportPusherPlugin",
    "ModelPluginConfig",
    "ModelPushResult",
    "ModelPusherPlugin",
    "PartialRegistrationError",
    "PluginRegistry",
    "PusherConfig",
    "PusherError",
    "PusherPluginConfig",
    "PusherPluginError",
    "PusherResult",
    "RegistrationResult",
    "default_registry",
    "push",
]
