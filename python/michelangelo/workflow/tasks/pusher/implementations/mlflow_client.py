"""MLflow-backed :class:`ModelRegistryClient` implementation.

Public module: construct :class:`MLflowRegistryClient` and pass it to
``push(..., registry_client=...)`` or list it in
``ModelPluginConfig.registry_clients`` to register pushed models in an
MLflow Model Registry. See
:class:`michelangelo.lib.model_manager.registry.client.ModelRegistryClient`
for the seam this implements.

Requires the ``mlflow`` optional dependency (the ``pusher-mlflow`` extra).
The import is deferred to first use, so merely importing this module does
not require mlflow to be installed.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING, Any

from michelangelo.lib.model_manager.registry.client import (
    ModelRegistryClient,
)
from michelangelo.lib.model_manager.registry.client import (
    RegisteredModel as MichelangeloModel,
)

if TYPE_CHECKING:
    from collections.abc import Mapping

# Model-version tag keys carrying the registration fields MLflow has no
# native column for. Prefixed so they cannot collide with user ``labels``
# (which map to plain MLflow tags); on collision the reserved value wins.
# The ``mlflow.`` tag namespace is reserved by MLflow itself and cannot
# collide with this one.
_TAG_DEPLOYABLE_ARTIFACT_URI = "michelangelo_deployable_artifact_uri"
_TAG_KIND = "michelangelo_kind"
_TAG_METADATA = "michelangelo_metadata"

_RESERVED_TAGS = frozenset({_TAG_DEPLOYABLE_ARTIFACT_URI, _TAG_KIND, _TAG_METADATA})


def _filter_literal(value: str) -> str | None:
    """Return ``value`` as a quoted MLflow filter string literal, or ``None``.

    MLflow's search filter grammar accepts single- or double-quoted string
    literals but (unlike SQL) supports no escaping *inside* a literal, so the
    quote style is chosen to avoid the quotes the value contains. A value
    containing both quote styles cannot be expressed at all — ``None`` tells
    the caller the search cannot be performed.
    """
    if "'" not in value:
        return f"'{value}'"
    if '"' not in value:
        return f'"{value}"'
    return None


class MLflowRegistryClient(ModelRegistryClient):
    """Registers pushed models in an MLflow Model Registry.

    Each :meth:`register_model` call creates the registered model (the
    version group) on first use and then a new model version whose
    ``source`` is the pushed artifact URI. Fields MLflow has no native
    column for travel as model-version tags: the deployable artifact URI
    and ``kind`` as dedicated tags, ``metadata`` JSON-encoded in one tag —
    except ``metadata["run_id"]``, which is passed to MLflow's native run
    linkage instead, per the ``ModelRegistryClient`` contract. ``labels``
    map directly to MLflow model-version tags. ``schema`` is accepted and
    ignored (MLflow has no schema field), also per the contract.

    The tracking server and credentials follow MLflow's native environment
    contract: ``MLFLOW_TRACKING_URI`` / ``MLFLOW_REGISTRY_URI`` (unless the
    corresponding constructor arguments are passed) and
    ``MLFLOW_TRACKING_USERNAME`` / ``MLFLOW_TRACKING_PASSWORD`` /
    ``MLFLOW_TRACKING_TOKEN``. Credentials are deliberately not accepted as
    constructor arguments so instances stay free of secrets.

    Example::

        from michelangelo.workflow.tasks.pusher import push
        from michelangelo.workflow.tasks.pusher.implementations import (
            MLflowRegistryClient,
        )

        result = push(
            config=pusher_config,
            artifacts=artifacts,
            registry_client=MLflowRegistryClient(
                tracking_uri="http://mlflow.example.com:5000",
            ),
        )
    """

    def __init__(
        self,
        tracking_uri: str | None = None,
        registry_uri: str | None = None,
    ) -> None:
        """Initialize the client.

        Args:
            tracking_uri: MLflow tracking server URI. Defaults to ``None``,
                which lets the MLflow client resolve ``MLFLOW_TRACKING_URI``
                from the environment.
            registry_uri: Registry server URI, for deployments where the
                model registry lives on a different server than tracking.
                Defaults to ``None``, which falls back to ``tracking_uri``
                (or ``MLFLOW_REGISTRY_URI`` from the environment).
        """
        self._tracking_uri = tracking_uri
        self._registry_uri = registry_uri

    def _client(self):
        """Construct an ``MlflowClient``, importing mlflow lazily.

        Raises:
            ImportError: If the ``mlflow`` optional dependency is not
                installed. The message tells the user how to fix their
                environment.
        """
        try:
            from mlflow.tracking import MlflowClient
        except ImportError as exc:
            raise ImportError(
                "MLflowRegistryClient requires the 'mlflow' package. Install"
                " it with the pusher-mlflow extra, e.g."
                " pip install 'michelangelo[pusher-mlflow]'"
            ) from exc
        return MlflowClient(
            tracking_uri=self._tracking_uri, registry_uri=self._registry_uri
        )

    def register_model(
        self,
        name: str,
        artifact_uri: str,
        deployable_artifact_uri: str | None = None,
        description: str | None = None,
        kind: str | None = None,
        schema: dict[str, Any] | None = None,
        labels: Mapping[str, str] | None = None,
        metadata: Mapping[str, Any] | None = None,
    ) -> MichelangeloModel:
        """Register a new model version in the MLflow Model Registry.

        Creates the registered model (version group) if it does not exist
        yet, then creates a model version with ``artifact_uri`` as its
        ``source``. ``schema`` is silently ignored — MLflow has no native
        schema field. ``metadata`` values must be JSON-serializable, per the
        ``ModelRegistryClient`` contract.

        Raises:
            ValueError: If ``name`` (or another argument) is rejected by
                MLflow's validation rules.
        """
        client = self._client()
        from mlflow.exceptions import MlflowException

        del schema  # No native schema field in MLflow; ignored per contract.

        try:
            client.create_registered_model(name)
        except MlflowException as exc:
            if exc.error_code == "INVALID_PARAMETER_VALUE":
                raise ValueError(str(exc)) from exc
            if exc.error_code != "RESOURCE_ALREADY_EXISTS":
                raise

        metadata_dict = dict(metadata or {})
        run_id = metadata_dict.get("run_id")
        native_run_id = run_id if isinstance(run_id, str) and run_id else None
        # run_id travels through MLflow's native run linkage, not the tag.
        stored_metadata = {
            key: value
            for key, value in metadata_dict.items()
            if not (key == "run_id" and native_run_id)
        }

        tags: dict[str, str] = dict(labels or {})
        if deployable_artifact_uri is not None:
            tags[_TAG_DEPLOYABLE_ARTIFACT_URI] = deployable_artifact_uri
        if kind is not None:
            tags[_TAG_KIND] = kind
        if stored_metadata:
            tags[_TAG_METADATA] = json.dumps(stored_metadata, sort_keys=True)

        try:
            model_version = client.create_model_version(
                name=name,
                source=artifact_uri,
                run_id=native_run_id,
                tags=tags,
                description=description,
            )
        except MlflowException as exc:
            if exc.error_code == "INVALID_PARAMETER_VALUE":
                raise ValueError(str(exc)) from exc
            raise

        return MichelangeloModel(
            name=name,
            version=str(model_version.version),
            registry_uri=f"models:/{name}/{model_version.version}",
            artifact_uri=artifact_uri,
            deployable_artifact_uri=deployable_artifact_uri,
            kind=kind,
            labels=dict(labels or {}),
            metadata=metadata_dict,
        )

    def get_model(self, name: str, version: str | None = None) -> MichelangeloModel:
        """Retrieve a model registration from the MLflow Model Registry.

        Args:
            name: Model name to look up.
            version: Specific version string. When ``None``, the highest
                version number is returned. Latest-version lookup goes
                through MLflow's search filter, whose grammar cannot express
                a name containing both a single and a double quote — for
                such (pathological) names, pass ``version`` explicitly.

        Raises:
            KeyError: If the model name or version is not found.
        """
        client = self._client()
        from mlflow.exceptions import MlflowException

        if version is not None:
            try:
                model_version = client.get_model_version(name=name, version=version)
            except MlflowException as exc:
                if exc.error_code == "RESOURCE_DOES_NOT_EXIST":
                    raise KeyError(
                        f"Model '{name}' version '{version}' not found."
                    ) from exc
                raise
            return self._to_registered_model(model_version)

        name_literal = _filter_literal(name)
        if name_literal is None:
            raise KeyError(
                f"Model name {name!r} contains both quote styles and cannot be"
                " expressed in an MLflow search filter; pass an explicit"
                " version instead."
            )
        model_versions = client.search_model_versions(
            filter_string=f"name = {name_literal}",
            order_by=["version_number DESC"],
            max_results=1,
        )
        if not model_versions:
            raise KeyError(f"Model '{name}' not found.")
        return self._to_registered_model(model_versions[0])

    def _to_registered_model(self, model_version) -> MichelangeloModel:
        """Map an MLflow ``ModelVersion`` entity back to the seam's dataclass."""
        tags = dict(model_version.tags or {})
        metadata_json = tags.get(_TAG_METADATA)
        metadata: dict[str, Any] = json.loads(metadata_json) if metadata_json else {}
        if model_version.run_id:
            metadata.setdefault("run_id", model_version.run_id)
        return MichelangeloModel(
            name=model_version.name,
            version=str(model_version.version),
            registry_uri=f"models:/{model_version.name}/{model_version.version}",
            artifact_uri=model_version.source,
            deployable_artifact_uri=tags.get(_TAG_DEPLOYABLE_ARTIFACT_URI),
            kind=tags.get(_TAG_KIND),
            labels={
                key: value for key, value in tags.items() if key not in _RESERVED_TAGS
            },
            metadata=metadata,
        )
