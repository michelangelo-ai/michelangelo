"""EvalReportPusherPlugin — enriches and persists an EvaluationReport."""

from __future__ import annotations

import json
import logging
import tempfile
import uuid
from typing import TYPE_CHECKING, Any

from google.protobuf.json_format import MessageToDict

from michelangelo.gen.api.v2.evaluation_report_pb2 import EvaluationReport
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.tasks.pusher.plugins.base import PusherPluginBase

if TYPE_CHECKING:
    from michelangelo.workflow.schema.pusher import EvalReportPluginConfig

_logger = logging.getLogger(__name__)

__all__ = ["EvalReportPusherPlugin"]


class EvalReportPusherPlugin(PusherPluginBase):
    """Plugin that enriches an EvaluationReport and writes it to a JSON file.

    Mirrors the internal plugin's enrichment pattern: resolves
    ``metadata.name`` (config override → proto field → auto-generated UUID),
    sets it on the proto, then serializes the enriched document to a local
    JSON file. The return dict exposes ``name`` and ``namespace`` so
    downstream tasks can reference the report by identity — matching the
    internal plugin's ``{"name": ..., "namespace": ...}`` return shape, with
    ``output_path`` added for the file-based OS implementation.

    Provider layers (e.g. Uber) subclass this and override ``execute()`` to
    replace the local file write with a gRPC push to their API server. All
    proto enrichment logic (name resolution, label injection, pipeline
    linkage) lives in the subclass.

    To integrate with MLflow, call
    ``mlflow.log_artifact(result["output_path"])`` after ``execute()``::

        result = plugin.execute()
        import mlflow
        mlflow.log_artifact(result["output_path"], artifact_path="eval_reports")

    Args:
        config: ``EvalReportPluginConfig`` with optional ``report_name`` (name
            override; takes precedence over ``artifact.metadata.name``) and
            ``extra_fields`` (arbitrary key-value pairs merged into the output
            JSON, e.g. CI run ID or git SHA — not part of the proto schema).
        artifact: An ``EvaluationReport`` protobuf message.
        storage_backend: Unused by this built-in implementation.
        registry_client: Unused by this built-in implementation.

    Raises:
        ConfigurationError: If ``artifact`` is ``None`` or not an
            ``EvaluationReport`` instance.

    Example::

        from michelangelo.gen.api.v2.evaluation_report_pb2 import (
            EvaluationReport,
            EvaluationReportSpec,
        )
        from michelangelo.workflow.schema.pusher import EvalReportPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
            EvalReportPusherPlugin,
        )

        spec = EvaluationReportSpec(title="Q1 Evaluation")
        report = EvaluationReport(spec=spec)

        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="q1-eval"),
            artifact=report,
        )
        result = plugin.execute()
        # result["name"]        → "q1-eval"
        # result["namespace"]   → ""  (set by provider subclasses)
        # result["output_path"] → "/tmp/michelangelo_reports_.../q1-eval.json"
    """

    def __init__(
        self,
        config: EvalReportPluginConfig,
        artifact: EvaluationReport | None = None,
        storage_backend: Any = None,
        registry_client: Any = None,
    ) -> None:
        """Validate that artifact is a non-None EvaluationReport.

        Args:
            config: Plugin configuration.
            artifact: An ``EvaluationReport`` protobuf message.
            storage_backend: Unused.
            registry_client: Unused.

        Raises:
            ConfigurationError: If ``artifact`` is ``None`` or not an
                ``EvaluationReport`` instance.
        """
        super().__init__(config, artifact, storage_backend, registry_client)
        if artifact is None:
            raise ConfigurationError(
                "EvalReportPusherPlugin requires an EvaluationReport artifact. "
                "Build one with EvaluationReport(spec=EvaluationReportSpec(...)) "
                "and pass it via artifact=."
            )
        if not isinstance(artifact, EvaluationReport):
            raise ConfigurationError(
                f"artifact must be an EvaluationReport; "
                f"got {type(artifact).__name__}. "
                "Use EvaluationReport(spec=EvaluationReportSpec(...)) to build one."
            )

    def execute(self) -> dict[str, Any]:
        """Enrich the EvaluationReport, serialize to JSON, and write to disk.

        Resolves ``metadata.name`` (config override → proto field →
        auto-generated), sets it on the proto, then serializes the enriched
        document. ``extra_fields`` are merged last and take precedence over
        proto fields on key collision.

        Returns:
            A dict with:

            - ``"name"``: resolved ``metadata.name`` of the report.
            - ``"namespace"``: ``metadata.namespace`` from the proto (empty
              string when not set; provider subclasses populate this from
              their project/namespace context).
            - ``"output_path"``: absolute path to the written JSON file.

        Raises:
            IOError: If the temp directory or JSON file cannot be written.
        """
        # Resolve name: config override → proto.metadata.name → auto-generate.
        # Then set it back on the proto so serialized output carries the name.
        name = (
            self._config.report_name
            or self._artifact.metadata.name
            or f"eval-report-{uuid.uuid4().hex[:8]}"
        )
        self._artifact.metadata.name = name

        document = {
            **MessageToDict(self._artifact, preserving_proto_field_name=True),
            **self._config.extra_fields,
        }

        output_dir = tempfile.mkdtemp(prefix="michelangelo_reports_")
        output_path = f"{output_dir}/{name}.json"

        with open(output_path, "w") as f:
            json.dump(document, f, indent=2)

        _logger.info(
            "EvalReportPusherPlugin: wrote report '%s' to '%s'.",
            name,
            output_path,
        )
        return {
            "name": name,
            "namespace": self._artifact.metadata.namespace,
            "output_path": output_path,
        }
