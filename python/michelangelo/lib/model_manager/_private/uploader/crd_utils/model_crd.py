from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import (
    ModelKind,
    PackageType,
)
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema
from uber.ai.michelangelo.sdk.model_manager._private.uploader.crd_utils.model_kind import convert_model_kind
from uber.ai.michelangelo.sdk.model_manager._private.uploader.crd_utils.package_type import convert_package_type
from uber.ai.michelangelo.sdk.model_manager._private.uploader.crd_utils.model_schema import set_model_schema
from uber.ai.michelangelo.sdk.model_manager._private.uploader.crd_utils.deployable_artifact_uri import (
    get_deployable_artifact_uri,
)
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import Model


def create_model_crd(
    project_name: str,
    model_name: str,
    package_type: Optional[str] = PackageType.TRITON,
    model_kind: Optional[str] = ModelKind.CUSTOM,
    model_family: Optional[str] = None,
    model_desc: Optional[str] = None,
    model_schema: Optional[ModelSchema] = None,
    revision_id: Optional[int] = 0,
    performance_report_name: Optional[str] = None,
    feature_report_name: Optional[str] = None,
    sealed: Optional[bool] = True,
    deployable_artifact_uri: Optional[list[str]] = None,
    raw_model_artifact_uri: Optional[list[str]] = None,
    labels: Optional[dict[str, str]] = None,
    annotations: Optional[dict[str, str]] = None,
    source_pipeline_run_name: Optional[str] = None,
    llm_vendor: Optional[str] = None,
    llm_fine_tuned_model_id: Optional[str] = None,
    training_framework: Optional[str] = None,
) -> Model:
    """
    Creates a model CRD object.

    Args:
        project_name (str): Name of the project.
        model_name (str): Name of the model.
        package_type (str): Type of the deployable model package.
        model_kind (str): Kind of the model.
        model_family (str): model family of the model.
        model_desc (str): Description of the model.
        model_schema (ModelSchema): Schema of the model.
        revision_id (int): ID of the revision.
        performance_report_name (str): Name of the performance report.
        feature_report_name (str): Name of the feature report.
        sealed (bool): Whether the model is sealed.
        deployable_artifact_uri (list[str]): List of deployable artifact URIs.
            If not provided, the default deployable artifact URI will be generated according to package type.
        raw_model_artifact_uri (list[str]): List of raw model artifact URIs.
            If not provided, the default raw model artifact URI will be used.
        labels (dict[str, str]): Labels for the model.
        annotations (dict[str, str]): Annotations for the model.
        source_pipeline_run_name (str): Source pipeline run for the model.
        llm_vendor (str): Vendor of the LLM model.
        llm_fine_tuned_model_id (str): ID of the finetuned LLM model.
        training_framework (str): The tool/library/interface to develop ML modeling with specified algorithm, such as tensorflow, pytorch, etc.

    Returns:
        Model: Model CRD object.
    """
    model = Model()

    model.metadata.namespace = project_name
    model.metadata.name = model_name

    model.spec.kind = convert_model_kind(model_kind)
    model.spec.package_type = convert_package_type(package_type)
    model.spec.sealed = sealed

    if model_family:
        model.spec.model_family.namespace = project_name
        model.spec.model_family.name = model_family

    if deployable_artifact_uri:
        model.spec.deployable_artifact_uri.extend(deployable_artifact_uri)
    else:
        deployable_artifact_uri = get_deployable_artifact_uri(project_name, model_name, str(revision_id or 0), package_type)
        model.spec.deployable_artifact_uri.append(deployable_artifact_uri)

    model.spec.revision_id = revision_id or 0

    if raw_model_artifact_uri:
        model.spec.model_artifact_uri.extend(raw_model_artifact_uri)

    if model_desc:
        model.spec.description = model_desc

    if model_schema:
        set_model_schema(model, model_schema)

    if performance_report_name:
        model.spec.performance_evaluation_report.name = performance_report_name

    if feature_report_name:
        model.spec.feature_evaluation_report.name = feature_report_name

    if labels:
        model.metadata.labels.update(labels)

    if annotations:
        model.metadata.annotations.update(annotations)

    if source_pipeline_run_name:
        model.spec.source_pipeline_run.namespace = project_name
        model.spec.source_pipeline_run.name = source_pipeline_run_name

    if llm_vendor:
        model.spec.llm_spec.vendor = llm_vendor

    if llm_fine_tuned_model_id:
        model.spec.llm_spec.fine_tuned_model_id = llm_fine_tuned_model_id

    if training_framework:
        model.spec.training_framework = training_framework

    return model
