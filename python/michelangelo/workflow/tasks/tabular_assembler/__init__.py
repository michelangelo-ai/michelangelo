"""Tabular assembler task — packages a trained tabular model for serving.

Exposes :func:`tabular_assembler`, the framework-dispatching entry point that
routes a raw trained model to the custom (Python-backend) or PyTorch/Lightning
assembler path based on ``ModelMetadata.training_framework`` and the
assembler configuration.

This task implements the "Package" stage of the model lifecycle:

    Train -> **Package** -> Register -> Deploy -> Serve

A training task produces a raw model artifact (weights plus
``ModelMetadata`` describing its framework, class, schema, and sample data).
``tabular_assembler`` turns that artifact into deployable and raw Triton
packages — optionally fusing it with a preceding native-transform model —
so it can be registered with the model registry, deployed to a serving
cluster, and queried through the model API. Consumers that only need the
packaging step can call ``tabular_assembler`` directly; it does not perform
registration, deployment, or serving itself.
"""

from __future__ import annotations

from michelangelo.workflow.tasks.tabular_assembler.task import tabular_assembler

__all__ = ["tabular_assembler"]
