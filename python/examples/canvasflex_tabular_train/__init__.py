"""CanvasFlex YAML example: a minimal tabular_train (Spark prep + Ray trainer).

Re-exports the workflow under the stable ``workflow_function`` alias so the
YAML can reference ``examples.canvasflex_tabular_train.workflow_function`` and
stay valid if the underlying function is renamed (mirrors the internal
workflow-def convention).
"""

from examples.canvasflex_tabular_train.workflow import (
    tabular_train as workflow_function,
)

__all__ = ["workflow_function"]
