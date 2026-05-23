"""EvalReportPusherPlugin — writes an evaluation report to a JSON file."""

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
    """Plugin that writes an evaluation report to a JSON file.

    Accepts either a plain ``dict`` of metrics or a typed ``EvaluationReport``
    protobuf message. Both are serialized to JSON and written to a temp
    directory. No storage backend or registry client is needed.

    Provider layers (e.g. Uber) subclass this and override ``execute()`` to
    post the report to a database or gRPC service instead.

    Args:
        config: ``EvalReportPluginConfig`` with optional ``report_name`` and
            ``extra_fields``.
        artifact: A ``dict`` of evaluation metrics or an ``EvaluationReport``
            protobuf message. ``None`` raises ``ConfigurationError``.
        storage_backend: Unused by this built-in implementation.
        registry_client: Unused by this built-in implementation.

    Raises:
        ConfigurationError: If ``artifact`` is ``None``, is an unsupported type,
            or contains the reserved key ``"_report_name"``.

    Example (dict artifact)::

        from michelangelo.workflow.schema.pusher import EvalReportPluginConfig
        from michelangelo.workflow.tasks.pusher.plugins.eval_report_plugin import (
            EvalReportPusherPlugin,
        )

        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="q1-eval"),
            artifact={"accuracy": 0.93, "f1": 0.91},
        )
        result = plugin.execute()
        # result["output_path"] → "/tmp/michelangelo_reports_.../q1-eval.json"

    Example (EvaluationReport proto artifact)::

        from michelangelo.gen.api.v2.evaluation_report_pb2 import (
            EvaluationReport,
            EvaluationReportSpec,
        )

        spec = EvaluationReportSpec(title="Q1 Evaluation")
        report = EvaluationReport(spec=spec)
        plugin = EvalReportPusherPlugin(
            config=EvalReportPluginConfig(report_name="q1-eval-proto"),
            artifact=report,
        )
        result = plugin.execute()
        # result["output_path"] → "/tmp/michelangelo_reports_.../q1-eval-proto.json"
    """

    def __init__(
        self,
        config: EvalReportPluginConfig,
        artifact: dict[str, Any] | EvaluationReport | None = None,
        storage_backend: Any = None,
        registry_client: Any = None,
    ) -> None:
        """Validate artifact presence, type, and reserved key constraint.

        Args:
            config: Plugin configuration.
            artifact: Evaluation report data as a ``dict`` or
                ``EvaluationReport`` protobuf message.
            storage_backend: Unused.
            registry_client: Unused.

        Raises:
            ConfigurationError: If ``artifact`` is ``None``, an unsupported
                type, or contains the reserved key ``"_report_name"``.
        """
        super().__init__(config, artifact, storage_backend, registry_client)
        if artifact is None:
            raise ConfigurationError(
                "EvalReportPusherPlugin requires a dict or EvaluationReport artifact. "
                "Pass the evaluation metrics via the artifact= argument."
            )
        if isinstance(artifact, EvaluationReport):
            self._artifact_dict: dict[str, Any] = MessageToDict(
                artifact,
                preserving_proto_field_name=True,
            )
        elif isinstance(artifact, dict):
            self._artifact_dict = artifact
        else:
            raise ConfigurationError(
                f"Artifact must be a dict or EvaluationReport; "
                f"got {type(artifact).__name__}. "
                "Pass a plain dict or an EvaluationReport protobuf message."
            )
        if _RESERVED_KEY in self._artifact_dict:
            raise ConfigurationError(
                f"Artifact must not contain the reserved key {_RESERVED_KEY!r}. "
                "It is added automatically by the plugin."
            )

    def execute(self) -> dict[str, Any]:
        """Write the evaluation report to a JSON file.

        Merges the artifact (normalized from dict or proto), ``config.extra_fields``,
        and ``_report_name`` into a single document. ``extra_fields`` take
        precedence over artifact keys on collision.

        Returns:
            A dict with:

            - ``"report_name"``: the assigned report name.
            - ``"output_path"``: absolute path to the written JSON file.
            - ``"num_keys"``: number of keys in the original artifact (not
              counting ``extra_fields`` or ``_report_name``).

        Raises:
            IOError: If the temp directory or JSON file cannot be written.
        """
        num_keys = len(self._artifact_dict)

        report_name = self._config.report_name or f"eval-report-{uuid.uuid4().hex[:8]}"

        document = {
            **self._artifact_dict,
            **self._config.extra_fields,
            _RESERVED_KEY: report_name,
        }

        output_dir = tempfile.mkdtemp(prefix="michelangelo_reports_")
        output_path = f"{output_dir}/{report_name}.json"

        with open(output_path, "w") as f:
            json.dump(document, f, indent=2)

        _logger.info(
            "EvalReportPusherPlugin: wrote %d-key report '%s' to '%s'.",
            num_keys,
            report_name,
            output_path,
        )
        return {
            "report_name": report_name,
            "output_path": output_path,
            "num_keys": num_keys,
        }
