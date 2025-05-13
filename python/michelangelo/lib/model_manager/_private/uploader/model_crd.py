from typing import Optional
from michelangelo.lib.model_manager.constants import (
    ModelKind,
    PackageType,
)
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.uploader.crd_utils import create_model_crd
from michelangelo.lib.model_manager._private.uploader.model_family import create_model_family
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import Model


def upload_model_crd(
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
    Uploads a model CRD to Michelangelo Unified API.

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
    from michelangelo.lib.model_manager._private.utils.api_client import APIClient

    if model_family:
        create_model_family(project_name, model_family)

    model = create_model_crd(
        project_name,
        model_name,
        package_type,
        model_kind,
        model_family,
        model_desc,
        model_schema,
        revision_id,
        performance_report_name,
        feature_report_name,
        sealed,
        deployable_artifact_uri,
        raw_model_artifact_uri,
        labels,
        annotations,
        source_pipeline_run_name,
        llm_vendor,
        llm_fine_tuned_model_id,
        training_framework,
    )

    if revision_id == 0:
        APIClient.ModelService.create_model(model)
        return model

    try:
        previous_model = APIClient.ModelService.get_model(name=model.metadata.name, namespace=model.metadata.namespace)
    except Exception:
        APIClient.ModelService.create_model(model)
    else:
        model.metadata.resourceVersion = previous_model.metadata.resourceVersion
        APIClient.ModelService.update_model(model)

    return model
