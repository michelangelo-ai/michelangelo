from typing import Optional
from uber.ai.michelangelo.sdk.api_client.v2beta1.utils import generate_random_name
from michelangelo.lib.model_manager.constants import ModelKind
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager.uploader.deployable_model import upload_deployable_model
from michelangelo.lib.model_manager.uploader.raw_model import upload_raw_model
from michelangelo.lib.model_manager._private.uploader import upload_model_crd
from michelangelo.lib.model_manager._private.utils.model_utils import (
    get_latest_model_revision_id,
    infer_model_package_type,
    infer_raw_model_package_type,
)
from uber.gen.code_uber_internal.uberai.michelangelo.api.v2beta1.model_proto import Model


def upload_model(
    project_name: str,
    deployable_model_path: str,
    raw_model_path: Optional[str] = None,
    model_name: Optional[str] = None,
    package_type: Optional[str] = None,
    timeout: Optional[str] = None,
    model_kind: Optional[str] = ModelKind.CUSTOM,
    model_family: Optional[str] = None,
    model_desc: Optional[str] = None,
    model_schema: Optional[ModelSchema] = None,
    revision_id: Optional[int] = None,
    performance_report_name: Optional[str] = None,
    feature_report_name: Optional[str] = None,
    sealed: Optional[bool] = True,
    labels: Optional[dict[str, str]] = None,
    annotations: Optional[dict[str, str]] = None,
    source_pipeline_run_name: Optional[str] = None,
    llm_vendor: Optional[str] = None,
    llm_fine_tuned_model_id: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> tuple[str, str, Model]:
    """
    Uploads a model to Michelangelo Unified API.

    Args:
        project_name (str): Name of the project.
        deployable_model_path (str): The path to the local dir with deployable model package to upload.
        raw_model_path (str): The path to the local dir with raw model package to upload.
        model_name (str): Name of the model. If not provided, a random name will be generated.
        package_type (str): Type of the deployable model package.
            If not provided, the package type will be infered from the model package.
        timeout (str): The timeout for uploading the model.
            Defaults to None, which results to 1ns in Terrablob.
            Format example 100ms, 0.7s, 10m, 2h. If no unit is specified, milliseconds are used.
        model_kind (str): Kind of the model.
        model_family (str): model family of the model.
        model_desc (str): Description of the model.
        model_schema (ModelSchema): Schema of the model.
        revision_id (int): ID of the revision. If not provided, the the revision ID will be the latest revision ID + 1.
        performance_report_name (str): Name of the performance report.
        feature_report_name (str): Name of the feature report.
        sealed (bool): Whether the model is sealed.
        labels (dict[str, str]): Labels for the model.
        annotations (dict[str, str]): Annotations for the model.
        source_pipeline_run_name (str): Source pipeline run for the model.
        llm_vendor (str): Vendor of the LLM model.
        llm_fine_tuned_model_id (str): ID of the finetuned LLM model
        source_entity (str): The source entity for terrablob command when uploading the model.
            Defaults to None, which results to "michelangelo-apiserver".

    Returns:
        tuple[str, str, Model]: The Terrablob path of the uploaded deployable model,
            the Terrablob path of the uploaded raw model, and the Model CRD object.
    """
    if not model_name:
        model_name = generate_random_name("model")

    if revision_id is None:
        revision_id = get_latest_model_revision_id(
            project_name,
            model_name,
        )
        revision_id = revision_id + 1 if revision_id >= 0 else 0

    if not package_type:
        package_type = infer_model_package_type(deployable_model_path)

    if raw_model_path:
        raw_model_package_type = infer_raw_model_package_type(raw_model_path)
        tb_raw_model_path = upload_raw_model(
            raw_model_path,
            project_name,
            model_name,
            timeout=timeout,
            revision_id=revision_id,
            source_entity=source_entity,
        )

        raw_model_artifact_uri = [tb_raw_model_path]
    else:
        tb_raw_model_path = None
        raw_model_artifact_uri = None
        raw_model_package_type = None

    tb_deployable_model_path = upload_deployable_model(
        deployable_model_path,
        project_name,
        model_name,
        package_type=package_type,
        timeout=timeout,
        revision_id=revision_id,
        source_entity=source_entity,
    )

    deployable_artifact_uri = [tb_deployable_model_path]

    model = upload_model_crd(
        project_name,
        model_name,
        package_type=package_type,
        model_kind=model_kind,
        model_family=model_family,
        model_desc=model_desc,
        model_schema=model_schema,
        revision_id=revision_id,
        performance_report_name=performance_report_name,
        feature_report_name=feature_report_name,
        sealed=sealed,
        deployable_artifact_uri=deployable_artifact_uri,
        raw_model_artifact_uri=raw_model_artifact_uri,
        labels=labels,
        annotations=annotations,
        source_pipeline_run_name=source_pipeline_run_name,
        llm_vendor=llm_vendor,
        llm_fine_tuned_model_id=llm_fine_tuned_model_id,
        training_framework=raw_model_package_type,
    )

    return tb_deployable_model_path, tb_raw_model_path, model
