"""Deploy a model produced by a child pipeline run."""

from __future__ import annotations

import logging
import time
from typing import Any

import grpc
from google.protobuf.any_pb2 import Any as AnyMessage
from google.protobuf.wrappers_pb2 import StringValue

from michelangelo.api.v2 import APIClient
from michelangelo.gen.api.list_pb2 import (
    CRITERION_OPERATOR_EQUAL,
    Criterion,
    CriterionOperation,
    ListOptionsExt,
)
from michelangelo.gen.api.options_pb2 import ResourceIdentifier
from michelangelo.gen.api.v2.deployment_pb2 import (
    DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
    DEPLOYMENT_STAGE_CLEAN_UP_FAILED,
    DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
    DEPLOYMENT_STAGE_ROLLBACK_FAILED,
    DEPLOYMENT_STAGE_ROLLOUT_COMPLETE,
    DEPLOYMENT_STAGE_ROLLOUT_FAILED,
    DEPLOYMENT_STATE_HEALTHY,
    Deployment,
    DeploymentSpec,
    DeploymentStage,
    DeploymentState,
    DeploymentStrategy,
    RollingUpdate,
)
from michelangelo.gen.api.v2.user_pb2 import UserInfo
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    CreateOptions,
    GetOptions,
    ObjectMeta,
    UpdateOptions,
)
from michelangelo.uniflow.core import star_plugin

log = logging.getLogger(__name__)

_DEFAULT_TIMEOUT_SECONDS = 10 * 365 * 24 * 60 * 60
_DEFAULT_POLL_SECONDS = 10
_UPDATE_ATTEMPTS = 3

_FAILED_STAGES = {
    DEPLOYMENT_STAGE_ROLLOUT_FAILED,
    DEPLOYMENT_STAGE_ROLLBACK_COMPLETE,
    DEPLOYMENT_STAGE_ROLLBACK_FAILED,
    DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE,
    DEPLOYMENT_STAGE_CLEAN_UP_FAILED,
}
_TRANSIENT_CODES = {
    grpc.StatusCode.UNAVAILABLE,
    grpc.StatusCode.DEADLINE_EXCEEDED,
    grpc.StatusCode.RESOURCE_EXHAUSTED,
}


def _packed_string(value: str) -> AnyMessage:
    packed = AnyMessage()
    packed.Pack(StringValue(value=value))
    return packed


def _model_query(pipeline_run_namespace: str, pipeline_run_name: str) -> ListOptionsExt:
    return ListOptionsExt(
        operation=CriterionOperation(
            criterion=[
                Criterion(
                    field_name="spec.source_pipeline_run.namespace",
                    match_value=_packed_string(pipeline_run_namespace),
                    operator=CRITERION_OPERATOR_EQUAL,
                ),
                Criterion(
                    field_name="spec.source_pipeline_run.name",
                    match_value=_packed_string(pipeline_run_name),
                    operator=CRITERION_OPERATOR_EQUAL,
                ),
            ]
        )
    )


def _resolve_model(namespace: str, pipeline_run_name: str, model_name: str | None):
    """Resolve exactly one current Model created by the requested pipeline run."""
    model_list = APIClient.ModelService.list_model(
        namespace=namespace,
        list_options_ext=_model_query(namespace, pipeline_run_name),
    )
    matches = []
    for model in model_list.items:
        source = model.spec.source_pipeline_run
        source_namespace = source.namespace or namespace
        if source.name != pipeline_run_name or source_namespace != namespace:
            continue
        if model_name is not None and model.metadata.name != model_name:
            continue
        matches.append(model)

    if not matches:
        suffix = f" and model {model_name!r}" if model_name is not None else ""
        raise RuntimeError(
            f"no model produced by pipeline run {namespace}/{pipeline_run_name}{suffix}"
        )
    if len(matches) > 1:
        names = sorted(model.metadata.name for model in matches)
        raise RuntimeError(
            f"pipeline run {namespace}/{pipeline_run_name} produced multiple models "
            f"{names}; pass model_name to select one"
        )
    return matches[0]


def _new_deployment(
    namespace: str,
    deployment_name: str,
    inference_server_name: str,
    model_name: str,
    actor: str | None,
) -> Deployment:
    spec = DeploymentSpec(
        desired_revision=ResourceIdentifier(namespace=namespace, name=model_name),
        inference_server=ResourceIdentifier(
            namespace=namespace, name=inference_server_name
        ),
        strategy=DeploymentStrategy(rolling=RollingUpdate()),
    )
    if actor:
        spec.owner.CopyFrom(UserInfo(name=actor))
    return Deployment(
        metadata=ObjectMeta(name=deployment_name, namespace=namespace),
        spec=spec,
    )


def _prepare_existing_deployment(
    deployment: Deployment,
    namespace: str,
    inference_server_name: str,
    model_name: str,
    actor: str | None,
) -> bool:
    target = deployment.spec.inference_server
    target_namespace = target.namespace or namespace
    if target.name != inference_server_name or target_namespace != namespace:
        raise RuntimeError(
            f"deployment {namespace}/{deployment.metadata.name} targets inference "
            f"server {target_namespace}/{target.name}, not "
            f"{namespace}/{inference_server_name}; refusing to retarget it"
        )

    desired = deployment.spec.desired_revision
    desired_namespace = desired.namespace or namespace
    changed = desired.name != model_name or desired_namespace != namespace
    if changed:
        desired.CopyFrom(ResourceIdentifier(namespace=namespace, name=model_name))
    if actor and deployment.spec.owner.name != actor:
        deployment.spec.owner.CopyFrom(UserInfo(name=actor))
        changed = True
    return changed


def _create_or_update_deployment(
    namespace: str,
    deployment_name: str,
    inference_server_name: str,
    model_name: str,
    actor: str | None,
) -> Deployment:
    service = APIClient.DeploymentService
    for attempt in range(_UPDATE_ATTEMPTS):
        try:
            existing = service.get_deployment(
                namespace=namespace,
                name=deployment_name,
                get_options=GetOptions(),
            )
        except grpc.RpcError as exc:
            if exc.code() != grpc.StatusCode.NOT_FOUND:
                raise
            deployment = _new_deployment(
                namespace,
                deployment_name,
                inference_server_name,
                model_name,
                actor,
            )
            try:
                return service.create_deployment(
                    deployment=deployment, create_options=CreateOptions()
                )
            except grpc.RpcError as create_exc:
                if (
                    create_exc.code() == grpc.StatusCode.ALREADY_EXISTS
                    and attempt + 1 < _UPDATE_ATTEMPTS
                ):
                    continue
                raise

        if not _prepare_existing_deployment(
            existing, namespace, inference_server_name, model_name, actor
        ):
            return existing
        try:
            return service.update_deployment(
                deployment=existing, update_options=UpdateOptions()
            )
        except grpc.RpcError as exc:
            if (
                exc.code() == grpc.StatusCode.FAILED_PRECONDITION
                and attempt + 1 < _UPDATE_ATTEMPTS
            ):
                continue
            raise
    raise RuntimeError(
        f"failed to create or update deployment {namespace}/{deployment_name}"
    )


def _same_revision(revision: ResourceIdentifier, namespace: str, name: str) -> bool:
    return revision.name == name and (revision.namespace or namespace) == namespace


def _deployment_result(deployment: Deployment, model_name: str) -> dict[str, Any]:
    return {
        "metadata": {
            "name": deployment.metadata.name,
            "namespace": deployment.metadata.namespace,
        },
        "model": {"name": model_name, "namespace": deployment.metadata.namespace},
        "status": {
            "state": DeploymentState.Name(deployment.status.state),
            "stage": DeploymentStage.Name(deployment.status.stage),
        },
    }


def _poll_deployment(
    namespace: str,
    deployment_name: str,
    model_name: str,
    timeout_seconds: int,
    poll_seconds: int,
) -> dict[str, Any]:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            deployment = APIClient.DeploymentService.get_deployment(
                namespace=namespace,
                name=deployment_name,
                get_options=GetOptions(),
            )
        except grpc.RpcError as exc:
            if exc.code() in _TRANSIENT_CODES:
                log.debug("transient error polling deployment: %s", exc)
                time.sleep(poll_seconds)
                continue
            raise RuntimeError(
                f"failed to get deployment {namespace}/{deployment_name}: "
                f"{exc.details()}"
            ) from exc

        stage = deployment.status.stage
        if stage in _FAILED_STAGES:
            detail = (
                f": {deployment.status.message}" if deployment.status.message else ""
            )
            raise RuntimeError(
                f"deployment {namespace}/{deployment_name} ended in "
                f"{DeploymentStage.Name(stage)}{detail}"
            )
        if (
            stage == DEPLOYMENT_STAGE_ROLLOUT_COMPLETE
            and deployment.status.state == DEPLOYMENT_STATE_HEALTHY
            and _same_revision(
                deployment.status.current_revision, namespace, model_name
            )
        ):
            return _deployment_result(deployment, model_name)
        time.sleep(poll_seconds)

    raise TimeoutError(
        f"deployment {namespace}/{deployment_name} did not roll out model "
        f"{model_name!r} within {timeout_seconds} seconds"
    )


@star_plugin("model.deploy_model")
def deploy_model(
    namespace: str,
    deployment_name: str,
    pipeline_run_name: str,
    inference_server_name: str,
    model_name: str | None = None,
    actor: str | None = None,
    timeout_seconds: int = 0,
    poll_seconds: int = _DEFAULT_POLL_SECONDS,
) -> dict[str, Any]:
    """Deploy the sole model produced by a completed pipeline run.

    ``model_name`` is optional when the pipeline run produced exactly one model and
    required when it produced more than one. Existing deployments are updated only
    when they already target the requested inference server.
    """
    required = {
        "namespace": namespace,
        "deployment_name": deployment_name,
        "pipeline_run_name": pipeline_run_name,
        "inference_server_name": inference_server_name,
    }
    for field, value in required.items():
        if not value:
            raise ValueError(f"{field} must be a non-empty string")
    if model_name == "":
        raise ValueError("model_name must be non-empty when provided")
    if timeout_seconds < 0:
        raise ValueError("timeout_seconds must be non-negative")
    if poll_seconds <= 0:
        raise ValueError("poll_seconds must be positive")
    if timeout_seconds == 0:
        timeout_seconds = _DEFAULT_TIMEOUT_SECONDS

    model = _resolve_model(namespace, pipeline_run_name, model_name)
    resolved_model_name = model.metadata.name
    _create_or_update_deployment(
        namespace,
        deployment_name,
        inference_server_name,
        resolved_model_name,
        actor,
    )
    return _poll_deployment(
        namespace,
        deployment_name,
        resolved_model_name,
        timeout_seconds,
        poll_seconds,
    )
