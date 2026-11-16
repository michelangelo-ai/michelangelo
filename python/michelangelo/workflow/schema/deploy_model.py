"""Configuration dataclasses for the ``deploy_model`` workflow task.

These classes are the canonical configuration schema for deploying a registered
model to a serving target. They mirror the structure of the Deployment API
(``proto/api/v2/deployment.proto``) using plain ``@dataclass`` types so every
class round-trips through the UniFlow ``DataclassCodec``:

* no ``init=False`` fields (they would be dropped on decode);
* no mutable default values (``... | None = None`` is used for collections);
* all enums are ``str`` enums, encoded by value.

The schema is intentionally free of validation logic. Cross-field rules such as
"exactly one target" and "exactly one strategy" are enforced by the task when a
new deployment is created, so that configs for updating an existing deployment
stay minimal.

Example:
    >>> config = DeployModelConfig(
    ...     deployment_name="my-deployment",
    ...     model_name="my-model-revision",
    ...     namespace="my-project",
    ... )
    >>> config.stage
    'production'
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

__all__ = [
    "BlastUpdateStrategy",
    "DeployModelConfig",
    "DeploymentDefinition",
    "DeviceIntegrityLevel",
    "DisaggregatedUpdateStep",
    "DisaggregatedUpdateStrategy",
    "Duration",
    "InferenceServerTarget",
    "LabelSelector",
    "LabelSelectorRequirement",
    "MobileTarget",
    "RedBlackUpdateStrategy",
    "ResourceIdentifier",
    "RollingUpdateStrategy",
    "ShadowSpec",
    "TargetType",
    "UserInfo",
    "ZonalUpdateStrategy",
]


class DeviceIntegrityLevel(str, Enum):
    """Device integrity levels required of mobile deployment targets.

    Attributes:
        STRONG: Hardware-backed attestation is required.
        STANDARD: Standard device attestation is sufficient.

    Example:
        >>> DeviceIntegrityLevel.STRONG.value
        'STRONG'
    """

    STRONG = "STRONG"
    STANDARD = "STANDARD"


class TargetType(str, Enum):
    """Kinds of deployment target a deployment definition can describe.

    Attributes:
        TARGET_TYPE_INFERENCE_SERVER: Deploy to an inference server.
        TARGET_TYPE_MOBILE: Deploy to mobile devices.

    Example:
        >>> TargetType.TARGET_TYPE_MOBILE.value
        'TARGET_TYPE_MOBILE'
    """

    TARGET_TYPE_INFERENCE_SERVER = "TARGET_TYPE_INFERENCE_SERVER"
    TARGET_TYPE_MOBILE = "TARGET_TYPE_MOBILE"


@dataclass
class InferenceServerTarget:
    """Inference server a model is deployed to.

    Attributes:
        namespace: Namespace of the inference server resource.
        name: Name of the inference server resource.
    """

    namespace: str
    name: str


@dataclass
class MobileTarget:
    """Mobile deployment target.

    Attributes:
        level: Minimum device integrity level required to receive the model.
    """

    level: DeviceIntegrityLevel


@dataclass
class ZonalUpdateStrategy:
    """Roll out one zone at a time.

    Attributes:
        rollout_period_in_seconds: Time to wait between zones. The backend
            default applies when ``None``.
    """

    rollout_period_in_seconds: float | None = None


@dataclass
class BlastUpdateStrategy:
    """Update every replica at once.

    Attributes:
        with_rollback_trigger: Whether an automatic rollback trigger is
            attached to the rollout. The backend default applies when ``None``.
        issue_link: Optional link to an issue or incident that justifies an
            emergency, unstaged rollout.
    """

    with_rollback_trigger: bool | None = None
    issue_link: str | None = None


@dataclass
class RollingUpdateStrategy:
    """Update a percentage of replicas per increment.

    Attributes:
        increment_percentage: Percentage of replicas updated per increment.
            Must be in the range 1 to 100 (checked when the task validates a
            new deployment).
    """

    increment_percentage: int


@dataclass
class RedBlackUpdateStrategy:
    """Bring up a full new set of replicas, then switch traffic over.

    Attributes:
        zonal_parallel: Whether zones are processed in parallel. The backend
            default applies when ``None``.
    """

    zonal_parallel: bool | None = None


@dataclass
class DisaggregatedUpdateStep:
    """One step of a disaggregated update.

    Attributes:
        environment: Environment the step applies to.
        strategy: Name of the update strategy used for that environment.
    """

    environment: str
    strategy: str


@dataclass
class DisaggregatedUpdateStrategy:
    """Update environments in an explicit sequence of steps.

    Attributes:
        steps: Ordered steps. Must be non-empty (checked when the task
            validates a new deployment).
    """

    steps: list[DisaggregatedUpdateStep]


@dataclass
class LabelSelectorRequirement:
    """A single label selector requirement.

    Attributes:
        key: Label key the requirement applies to.
        operator: One of ``In``, ``NotIn``, ``Exists`` or ``DoesNotExist``.
        values: Label values. Used by the ``In`` and ``NotIn`` operators and
            empty for ``Exists`` and ``DoesNotExist``.
    """

    key: str
    operator: str
    values: list[str] | None = None


@dataclass
class LabelSelector:
    """Label selector that narrows the environments or partitions targeted.

    Attributes:
        match_labels: Labels that must match exactly.
        match_expressions: Additional label requirements. All must hold.
    """

    match_labels: dict[str, str] | None = None
    match_expressions: list[LabelSelectorRequirement] | None = None


@dataclass
class ResourceIdentifier:
    """Reference to a resource by namespace and name.

    Attributes:
        namespace: Namespace of the resource.
        name: Name of the resource.
    """

    namespace: str
    name: str


@dataclass
class Duration:
    """A span of time.

    Attributes:
        seconds: Whole seconds.
        nanos: Nanoseconds added to ``seconds``.
    """

    seconds: int | None = None
    nanos: int | None = None


@dataclass
class ShadowSpec:
    """Shadow deployment settings.

    Attributes:
        traffic_percentage: Share of traffic mirrored to the shadow
            deployment, from 1 to 100.
        duration: How long the shadow deployment runs.
        enabled: Whether shadowing is on.
    """

    traffic_percentage: float | None = None
    duration: Duration | None = None
    enabled: bool | None = None


@dataclass
class DeploymentDefinition:
    """Describes what kind of deployment this is.

    Attributes:
        type: Kind of target the deployment serves.
        sub_type: Free-form refinement of ``type``.
    """

    type: TargetType | None = None
    sub_type: str | None = None


@dataclass
class UserInfo:
    """Identity that owns a deployment.

    Attributes:
        name: Name of the owning user or service account.
        proxy_user: Optional user on whose behalf the owner acts.
    """

    name: str
    proxy_user: str | None = None


@dataclass
class DeployModelConfig:
    """Configuration for the ``deploy_model`` task.

    ``deployment_name``, ``model_name`` and ``namespace`` are always required.
    The target, strategy, definition and owner fields are only needed when the
    deployment does not exist yet; the task validates them on the create path.

    Attributes:
        deployment_name: Name of the deployment to create or update.
        model_name: Name of the model revision the deployment should serve.
        namespace: Namespace containing the deployment and model.
        inference_server: Inference server target. Set exactly one of
            ``inference_server`` and ``mobile`` for new deployments.
        mobile: Mobile target.
        zonal_strategy: Zonal update strategy. Set exactly one strategy field
            for new deployments.
        blast_strategy: Blast update strategy.
        rolling_strategy: Rolling update strategy.
        red_black_strategy: Red-black update strategy.
        disaggregated_strategy: Disaggregated update strategy.
        definition: Deployment definition.
        owner: Owner of a new deployment.
        selector: Label selector narrowing the deployment scope.
        model_family: Model family the deployment belongs to.
        shadow_spec: Shadow deployment settings.
        resource_links: Named links to related resources, such as dashboards.
        stage: Deployment stage recorded as a label, for example
            ``production``, ``staging`` or ``development``.
        additional_labels: Extra labels applied to a new deployment.
        wait: Whether to wait for the rollout to finish. When ``False`` the
            task returns once the deployment is created or updated.
        timeout_seconds: Upper bound in seconds on waiting for the rollout.
            Must be greater than zero. Defaults to 20 hours.
        deployment_template: Name of an existing deployment to clone when
            creating a new one, as an alternative to the structured fields
            above.

    Example:
        >>> config = DeployModelConfig(
        ...     deployment_name="ranker",
        ...     model_name="ranker-v3",
        ...     namespace="my-project",
        ...     inference_server=InferenceServerTarget(
        ...         namespace="my-project", name="ranker-server"
        ...     ),
        ...     rolling_strategy=RollingUpdateStrategy(increment_percentage=25),
        ... )
        >>> config.wait
        False
    """

    deployment_name: str
    model_name: str
    namespace: str

    inference_server: InferenceServerTarget | None = None
    mobile: MobileTarget | None = None

    zonal_strategy: ZonalUpdateStrategy | None = None
    blast_strategy: BlastUpdateStrategy | None = None
    rolling_strategy: RollingUpdateStrategy | None = None
    red_black_strategy: RedBlackUpdateStrategy | None = None
    disaggregated_strategy: DisaggregatedUpdateStrategy | None = None

    definition: DeploymentDefinition | None = None
    owner: UserInfo | None = None

    selector: LabelSelector | None = None
    model_family: ResourceIdentifier | None = None
    shadow_spec: ShadowSpec | None = None
    resource_links: dict[str, str] | None = None

    stage: str = "production"
    additional_labels: dict[str, str] | None = None

    wait: bool = False
    timeout_seconds: int = 72000
    deployment_template: str | None = None
