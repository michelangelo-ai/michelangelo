"""Built-in pusher plugins for the michelangelo workflow task package.

Importing this module populates ``default_registry`` with the three built-in
plugins. ``push()`` imports ``pusher.plugins`` before resolving the registry,
so the registrations are guaranteed to be present on first use.
"""

from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
from michelangelo.workflow.tasks.pusher.plugins.dataset_plugin import (
    DatasetPusherPlugin,
)
from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
    EvalReportPusherPlugin,
)
from michelangelo.workflow.tasks.pusher.plugins.model_plugin import (
    ModelPusherPlugin,
    ModelPushResult,
    PartialRegistrationError,
    RegistrationResult,
)
from michelangelo.workflow.tasks.pusher.registry import default_registry
from michelangelo.workflow.variables import DatasetVariable
from michelangelo.workflow.variables.types import AssembledModel

__all__ = [
    "DatasetPusherPlugin",
    "EvalReportPusherPlugin",
    "ModelPushResult",
    "ModelPusherPlugin",
    "PartialRegistrationError",
    "RegistrationResult",
]

# Populate default_registry. Done here (not in registry.py) to avoid a
# circular import: registry.py ← plugins/base.py ← plugins/__init__.py.
default_registry.register("model_plugin", ModelPusherPlugin, AssembledModel)
default_registry.register("dataset_plugin", DatasetPusherPlugin, DatasetVariable)
default_registry.register(
    "eval_report_plugin", EvalReportPusherPlugin, EvaluationReport
)
