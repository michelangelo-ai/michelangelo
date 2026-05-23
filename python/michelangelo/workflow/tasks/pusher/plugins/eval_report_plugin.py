"""EvalReportPusherPlugin — writes an EvaluationReport to a JSON file."""

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

_RESERVED_KEY = "_report_name"


class EvalReportPusherPlugin(PusherPluginBase):
    """Plugin that serializes an EvaluationReport proto to a local JSON file.

    The ``EvaluationReport`` schema (title, charts, filters, data source,
    pipeline references) is Michelangelo's canonical evaluation document type.
    This built-in implementation writes it as JSON to a temp directory and
    returns the output path. Provider layers (e.g. internal Uber) subclass
    this and override ``execute()`` to push the report to a database or gRPC
    service instead.

    To integrate with MLflow, call ``mlflow.log_artifact(result["output_path"])``
    after ``execute()`` in a subclass or post-processing step.

    Args:
        config: ``EvalReportPluginConfig`` with optional ``report_name`` and
            ``extra_fields`` merged into the serialized document.
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
                f"artifact must be an EvaluationReport; got {type(artifact).__name__}. "
                "Use EvaluationReport(spec=EvaluationReportSpec(...)) to build one."
            )

    def execute(self) -> dict[str, Any]:
        """Serialize the EvaluationReport to JSON and write to a temp directory.

        Converts the proto to a snake_case JSON dict via ``MessageToDict``,
        merges ``config.extra_fields`` (extra fields take precedence), injects
        ``_report_name``, and writes the document to a file under a fresh
        ``michelangelo_reports_`` temp directory.

        Returns:
            A dict with:

            - ``"report_name"``: the assigned report name (from config or
              auto-generated as ``"eval-report-{uuid8}"``).
            - ``"output_path"``: absolute path to the written JSON file.
            - ``"num_keys"``: number of top-level keys in the serialized proto
              (not counting ``extra_fields`` or ``_report_name``).

        Raises:
            IOError: If the temp directory or JSON file cannot be written.
        """
        artifact_dict = MessageToDict(
            self._artifact,
            preserving_proto_field_name=True,
        )

        report_name = (
            self._config.report_name or f"eval-report-{uuid.uuid4().hex[:8]}"
        )

        document = {
            **artifact_dict,
            **self._config.extra_fields,
            _RESERVED_KEY: report_name,
        }

        output_dir = tempfile.mkdtemp(prefix="michelangelo_reports_")
        output_path = f"{output_dir}/{report_name}.json"

        with open(output_path, "w") as f:
            json.dump(document, f, indent=2)

        _logger.info(
            "EvalReportPusherPlugin: wrote report '%s' to '%s'.",
            report_name,
            output_path,
        )
        return {
            "report_name": report_name,
            "output_path": output_path,
            "num_keys": len(artifact_dict),
        }
