"""MLflow-backed :class:`~schema.ExperimentStore` implementation for auto-resume.

Public module: users import and construct :class:`MlflowExperimentStore` and
pass it as ``LightningTrainerParam.experiment_store`` to opt into auto-resume
backed by an MLflow tracking server instead of a filesystem marker file.
See :class:`michelangelo.lib.trainer.torch.pytorch_lightning.schema.ExperimentStore`
for the seam it implements and
:class:`~experiment_store.FsspecExperimentStore` for the filesystem default.

Requires the ``mlflow`` optional dependency (the ``trainer-mlflow`` extra).
The import is deferred to first use, so merely importing this module does not
require mlflow to be installed.
"""

from __future__ import annotations

import logging

_logger = logging.getLogger(__name__)

# Tag keys carrying the marker payload on each MLflow run. Underscore-joined
# (no dots) so they never need identifier quoting in a search ``filter_string``.
# The ``mlflow.`` prefix is reserved by MLflow for system tags; this namespace
# cannot collide with it.
_TAG_SCHEMA_VERSION = "michelangelo_resume_schema_version"
_TAG_RUN_NAME = "michelangelo_resume_run_name"
_TAG_STORAGE_PATH = "michelangelo_resume_storage_path"
_TAG_EXPERIMENT_PATH = "michelangelo_resume_experiment_path"

# How many newest candidate runs to fetch per lookup. The tag filter should
# already narrow to exact matches; the small window only exists so the
# client-side identity re-check (see ``locate_resumable``) has alternatives if
# a same-identity run carries an empty payload.
_SEARCH_WINDOW = 5


def _filter_literal(value: str) -> str | None:
    """Return ``value`` as a quoted MLflow filter string literal, or ``None``.

    MLflow's filter grammar accepts single- or double-quoted string literals
    but (unlike SQL) supports no escaping *inside* a literal, so the quote
    style is chosen to avoid the quotes the value contains. A value containing
    both quote styles cannot be expressed at all — ``None`` signals the caller
    to treat the lookup as "nothing to resume".
    """
    if "'" not in value:
        return f"'{value}'"
    if '"' not in value:
        return f'"{value}"'
    return None


class MlflowExperimentStore:
    """MLflow-backed :class:`~schema.ExperimentStore` using marker runs.

    Records each training run's experiment directory as a small, immediately
    terminated MLflow run ("marker run") in a dedicated MLflow experiment,
    tagged with the stable ``(storage_path, run_name)`` identity. A re-run with
    the same ``RunConfig(storage_path=..., name=...)`` locates the most recent
    marker for that identity and resumes from the directory it records.

    Unlike :class:`~experiment_store.FsspecExperimentStore`, MLflow offers no
    deterministic key-value lookup or atomic overwrite: every :meth:`track`
    creates a *new* marker run, and :meth:`locate_resumable` returns the most
    recent one (ordered by start time). "Most recent wins" gives the same
    end-user semantics as an overwrite, at the cost of one marker run per
    training attempt accumulating in the MLflow experiment — a deliberate v1
    tradeoff, kept simple because :meth:`track` runs on worker rank 0 only.

    The tracking server and credentials follow MLflow's native environment
    contract: ``MLFLOW_TRACKING_URI`` (unless ``tracking_uri`` is passed) and
    ``MLFLOW_TRACKING_USERNAME`` / ``MLFLOW_TRACKING_PASSWORD`` /
    ``MLFLOW_TRACKING_TOKEN``. Credentials are deliberately not accepted as
    constructor arguments: the store is pickled into ``train_loop_config`` and
    shipped to Ray workers, and that config is logged, so secrets must stay in
    the environment.

    Neither method raises: :meth:`track` logs and swallows any failure
    (including mlflow not being installed), and :meth:`locate_resumable`
    returns ``None`` when there is nothing to resume or the server cannot be
    reached.

    Example::

        from michelangelo.lib.trainer.torch.pytorch_lightning import (
            LightningTrainerParam,
            MlflowExperimentStore,
        )

        param = LightningTrainerParam(
            create_model_fn=my_model_factory,
            create_model_fn_kwargs={},
            train_data=train_ds,
            val_data=val_ds,
            experiment_store=MlflowExperimentStore(
                tracking_uri="http://mlflow.example.com",
                experiment_name="team-x-resume",
            ),
        )

    Teams sharing one MLflow server should pass a distinct ``experiment_name``
    per project so unrelated projects' markers stay in separate experiments.
    """

    _SCHEMA_VERSION = 1

    def __init__(
        self,
        tracking_uri: str | None = None,
        experiment_name: str = "michelangelo-resume",
    ) -> None:
        """Initialize the store.

        Args:
            tracking_uri: MLflow tracking server URI. Defaults to ``None``,
                which lets the MLflow client resolve ``MLFLOW_TRACKING_URI``
                from the environment (on the driver and on workers alike).
            experiment_name: Name of the MLflow experiment holding the marker
                runs. Created on first :meth:`track` if absent.
        """
        self._tracking_uri = tracking_uri
        self._experiment_name = experiment_name

    def _client(self):
        """Construct an ``MlflowClient``, importing mlflow lazily.

        Raises:
            ImportError: If the ``mlflow`` optional dependency is not
                installed (callers swallow this per the never-raise contract,
                but the message tells the user how to fix their environment).
        """
        try:
            from mlflow.tracking import MlflowClient
        except ImportError as exc:
            raise ImportError(
                "MlflowExperimentStore requires the 'mlflow' package. Install"
                " it with the trainer-mlflow extra, e.g."
                " pip install 'michelangelo[trainer-mlflow]'"
            ) from exc
        return MlflowClient(tracking_uri=self._tracking_uri)

    def _ensure_experiment(self, client) -> str:
        """Return the marker experiment's id, creating the experiment if absent.

        Tolerates the create/create race (two first-ever runs tracking
        concurrently): a failed ``create_experiment`` falls back to re-reading
        by name.
        """
        experiment = client.get_experiment_by_name(self._experiment_name)
        if experiment is not None:
            return experiment.experiment_id
        try:
            return client.create_experiment(self._experiment_name)
        except Exception:
            # Lost the creation race (or a transient error) — re-read; if the
            # experiment still does not exist, let the original caller's
            # exception handling deal with it.
            experiment = client.get_experiment_by_name(self._experiment_name)
            if experiment is None:
                raise
            return experiment.experiment_id

    def track(self, *, storage_path: str, run_name: str, experiment_path: str) -> None:
        """Create a marker run recording this run's experiment directory.

        Best-effort: any failure (unreachable server, missing mlflow package,
        auth error) is logged and swallowed so a failed marker write can never
        fail an otherwise-successful training run. The marker run is
        terminated immediately — it exists only to carry tags.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.
            experiment_path: Absolute path (scheme-qualified for remote
                filesystems, e.g. ``s3://bucket/runs/my_run``) of this run's
                Ray Train experiment directory.

        Returns:
            ``None``.
        """
        try:
            if not storage_path or not run_name:
                _logger.debug(
                    "Not tracking resume marker: empty storage_path or run_name"
                )
                return
            client = self._client()
            experiment_id = self._ensure_experiment(client)
            run = client.create_run(
                experiment_id,
                tags={
                    _TAG_SCHEMA_VERSION: str(self._SCHEMA_VERSION),
                    _TAG_RUN_NAME: run_name,
                    _TAG_STORAGE_PATH: storage_path,
                    _TAG_EXPERIMENT_PATH: experiment_path,
                },
                run_name=run_name,
            )
            client.set_terminated(run.info.run_id, status="FINISHED")
        except Exception:  # best-effort: never fail training
            _logger.warning("MlflowExperimentStore.track failed", exc_info=True)

    def locate_resumable(self, *, storage_path: str, run_name: str) -> str | None:
        """Return the most recent marker's experiment path, or ``None``.

        Searches the marker experiment for runs tagged with this exact
        ``(storage_path, run_name)`` identity, newest first, and returns the
        recorded experiment path of the first match. Returns ``None`` (never
        raises) when the experiment does not exist yet, no marker matches, or
        the server cannot be reached.

        Args:
            storage_path: The driver's ``RunConfig.storage_path``.
            run_name: The driver's ``RunConfig.name``.

        Returns:
            The recorded candidate experiment directory, or ``None``.
        """
        try:
            if not storage_path or not run_name:
                _logger.debug("No resumable marker: empty storage_path or run_name")
                return None
            run_name_literal = _filter_literal(run_name)
            storage_path_literal = _filter_literal(storage_path)
            if run_name_literal is None or storage_path_literal is None:
                _logger.debug(
                    "No resumable marker: identity contains both quote styles"
                    " and cannot be expressed in an MLflow search filter"
                )
                return None
            client = self._client()
            experiment = client.get_experiment_by_name(self._experiment_name)
            if experiment is None:
                return None
            filter_string = (
                f"tags.{_TAG_RUN_NAME} = {run_name_literal}"
                f" and tags.{_TAG_STORAGE_PATH} = {storage_path_literal}"
            )
            runs = client.search_runs(
                experiment_ids=[experiment.experiment_id],
                filter_string=filter_string,
                order_by=["attributes.start_time DESC"],
                max_results=_SEARCH_WINDOW,
            )
            for run in runs:
                tags = run.data.tags
                # Re-check the identity client-side so a filter-escaping edge
                # case can never resume from another run's directory.
                if (
                    tags.get(_TAG_RUN_NAME) == run_name
                    and tags.get(_TAG_STORAGE_PATH) == storage_path
                ):
                    experiment_path = tags.get(_TAG_EXPERIMENT_PATH)
                    if experiment_path:
                        return experiment_path
            return None
        except Exception:  # unexpected client/server error -> nothing to resume
            _logger.warning(
                "MlflowExperimentStore.locate_resumable failed", exc_info=True
            )
            return None
