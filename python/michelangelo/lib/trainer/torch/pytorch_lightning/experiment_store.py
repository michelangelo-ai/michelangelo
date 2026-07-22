"""Default :class:`~schema.ExperimentStore` implementation for auto-resume.

Public module: users import and construct :class:`FsspecExperimentStore` and
pass it as ``LightningTrainerParam.experiment_store`` to opt into auto-resume.
See :class:`michelangelo.lib.trainer.torch.pytorch_lightning.schema.ExperimentStore`
for the seam it implements.
"""

from __future__ import annotations

import json
import logging
from datetime import datetime, timezone

from fsspec.core import url_to_fs

_logger = logging.getLogger(__name__)


class FsspecExperimentStore:
    """Filesystem-backed :class:`~schema.ExperimentStore` using an fsspec marker file.

    Persists a small JSON marker at a deterministic path so a re-run with the
    same ``RunConfig(storage_path=..., name=...)`` can locate and resume the
    previous run's experiment directory. Works with any fsspec scheme the
    ``storage_path`` selects (local, ``s3://``, ``gs://``, ...).

    The marker lives at ``{storage_path}/.michelangelo_resume/{run_name}.json``
    rather than inside the Ray experiment directory, because that path is
    derivable purely from the stable ``(storage_path, run_name)`` identity and
    does not depend on Ray's (possibly timestamp-suffixed) experiment directory
    name. It inherits the same permissions and lifecycle as the run data it sits
    beside.

    Neither method raises: :meth:`track` logs and swallows any write failure,
    and :meth:`locate_resumable` returns ``None`` on a missing, corrupt, or
    unreadable marker. Staleness is not treated as an error — the trainer runs
    the recorded path through ``TorchTrainer.can_restore()`` and starts fresh if
    it does not point at a restorable checkpoint.

    Attributes:
        _MARKER_DIR: Subdirectory (under ``storage_path``) holding marker files.
        _SCHEMA_VERSION: Marker payload schema version, for forward evolution.
    """

    _MARKER_DIR = ".michelangelo_resume"
    _SCHEMA_VERSION = 1

    def __init__(self, storage_options: dict | None = None) -> None:
        """Initialize the store.

        Args:
            storage_options: Optional keyword arguments forwarded to
                ``fsspec.core.url_to_fs`` (e.g. credentials or endpoint
                overrides for the backing filesystem). Kept as a plain dict so
                the store remains picklable for transport to Ray workers.
        """
        self._storage_options = storage_options or {}

    def _marker_path(self, storage_path: str, run_name: str) -> str:
        """Return the deterministic marker path for ``(storage_path, run_name)``."""
        return f"{storage_path.rstrip('/')}/{self._MARKER_DIR}/{run_name}.json"

    def track(self, *, storage_path: str, run_name: str, experiment_path: str) -> None:
        """Write the marker recording this run's experiment directory.

        Best-effort: any failure is logged and swallowed so a failed marker
        write can never fail an otherwise-successful training run.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.
            experiment_path: Absolute path of this run's Ray Train experiment
                directory.

        Returns:
            ``None``.
        """
        try:
            fs, path = url_to_fs(
                self._marker_path(storage_path, run_name), **self._storage_options
            )
            payload = json.dumps(
                {
                    "schema_version": self._SCHEMA_VERSION,
                    "run_name": run_name,
                    "experiment_path": experiment_path,
                    "written_at": datetime.now(timezone.utc).isoformat(),
                }
            )
            fs.makedirs(path.rsplit("/", 1)[0], exist_ok=True)
            with fs.open(path, "w") as f:
                f.write(payload)
        except Exception:  # best-effort: never fail training
            _logger.warning("FsspecExperimentStore.track failed", exc_info=True)

    def locate_resumable(self, *, storage_path: str, run_name: str) -> str | None:
        """Read the marker and return the recorded experiment path, or ``None``.

        Returns ``None`` (never raises) when the marker is missing, corrupt, or
        unreadable, or when it carries no ``experiment_path``.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.

        Returns:
            The recorded candidate experiment directory, or ``None``.
        """
        try:
            fs, path = url_to_fs(
                self._marker_path(storage_path, run_name), **self._storage_options
            )
            if not fs.exists(path):
                return None
            with fs.open(path, "r") as f:
                data = json.loads(f.read())
            return data.get("experiment_path") or None
        except Exception:  # missing / corrupt / unreadable -> nothing to resume
            _logger.warning(
                "FsspecExperimentStore.locate_resumable failed", exc_info=True
            )
            return None
