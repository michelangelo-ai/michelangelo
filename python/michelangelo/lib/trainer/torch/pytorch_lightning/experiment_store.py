"""Default :class:`~schema.ExperimentStore` implementation for auto-resume.

Public module: users import and construct :class:`FsspecExperimentStore` and
pass it as ``LightningTrainerParam.experiment_store`` to opt into auto-resume.
See :class:`michelangelo.lib.trainer.torch.pytorch_lightning.schema.ExperimentStore`
for the seam it implements.
"""

from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone

from fsspec.core import url_to_fs

_logger = logging.getLogger(__name__)


class _InvalidRunNameError(ValueError):
    """Raised internally when a ``run_name`` is unsafe for use in a marker path."""


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
    unreadable marker. Staleness is not treated as an error — the trainer only
    seeds a resume when the recorded directory still holds a restorable Ray
    Train checkpoint, and otherwise starts fresh.

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
        """Return the deterministic marker path for ``(storage_path, run_name)``.

        Args:
            storage_path: The storage root under which markers are kept.
            run_name: The stable run identity. Used verbatim as the marker
                filename stem, so it must not contain path separators.

        Returns:
            The marker path ``{storage_path}/.michelangelo_resume/{run_name}.json``.

        Raises:
            _InvalidRunNameError: If ``run_name`` is empty or contains a path
                separator or ``..`` component that could escape the marker
                directory.
        """
        if (
            not run_name
            or "/" in run_name
            or "\\" in run_name
            or run_name in (".", "..")
        ):
            raise _InvalidRunNameError(
                f"run_name must be a single path-safe component, got {run_name!r}"
            )
        return f"{storage_path.rstrip('/')}/{self._MARKER_DIR}/{run_name}.json"

    def track(self, *, storage_path: str, run_name: str, experiment_path: str) -> None:
        """Write the marker recording this run's experiment directory.

        Best-effort: any failure is logged and swallowed so a failed marker
        write can never fail an otherwise-successful training run. The payload
        is written to a temporary sibling file and atomically renamed into
        place, so a crash mid-write can never leave a partial/corrupt marker
        that a later :meth:`locate_resumable` would read.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.
            experiment_path: Absolute path (scheme-qualified for remote
                filesystems, e.g. ``s3://bucket/runs/my_run``) of this run's Ray
                Train experiment directory.

        Returns:
            ``None``.
        """
        try:
            marker = self._marker_path(storage_path, run_name)
            fs, path = url_to_fs(marker, **self._storage_options)
            payload = json.dumps(
                {
                    "schema_version": self._SCHEMA_VERSION,
                    "run_name": run_name,
                    "experiment_path": experiment_path,
                    "written_at": datetime.now(timezone.utc).isoformat(),
                }
            )
            parent = path.rsplit("/", 1)[0]
            fs.makedirs(parent, exist_ok=True)
            # Write to a unique temp sibling then atomically rename into place so
            # a crash mid-write cannot leave a partial marker behind.
            tmp_path = f"{path}.{uuid.uuid4().hex}.tmp"
            with fs.open(tmp_path, "w") as f:
                f.write(payload)
            fs.mv(tmp_path, path)
        except Exception:  # best-effort: never fail training
            _logger.warning("FsspecExperimentStore.track failed", exc_info=True)

    def locate_resumable(self, *, storage_path: str, run_name: str) -> str | None:
        """Read the marker and return the recorded experiment path, or ``None``.

        Returns ``None`` (never raises) when the marker is missing, corrupt, or
        unreadable, or when it carries no ``experiment_path``. A missing or
        corrupt marker is the normal "nothing to resume yet" case and is logged
        at ``DEBUG``; only genuinely unexpected filesystem errors are logged at
        ``WARNING``.

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
        except (json.JSONDecodeError, _InvalidRunNameError) as exc:
            # A corrupt marker or an unusable run_name means "nothing to
            # resume" — an expected, benign outcome, not an error.
            _logger.debug("No resumable marker for run_name=%r: %s", run_name, exc)
            return None
        except Exception:  # unexpected filesystem error -> nothing to resume
            _logger.warning(
                "FsspecExperimentStore.locate_resumable failed", exc_info=True
            )
            return None
