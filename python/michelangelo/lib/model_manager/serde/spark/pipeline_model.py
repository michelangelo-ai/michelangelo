import os
import uuid
import tempfile
from typing import Optional
from pyspark.ml import PipelineModel
from uber.ai.michelangelo.sdk.compat.pyspark.utils.session import get_spark_session
from uber.ai.michelangelo.sdk.compat.pyspark.ml.pipeline import MichelangeloPipelineModel
from michelangelo._internal.gateways.hdfs_gateway import (
    create_dir_in_hdfs,
    upload_to_hdfs,
    download_from_hdfs,
)
from michelangelo.lib.model_manager._private.constants.hdfs_paths import (
    HDFS_TMP_MODELS_DIR,
)


def load_spark_pipeline_model(model_path: str) -> PipelineModel:
    """
    Load a Spark pipeline model from the given path.

    Args:
        model_path: The local path to the model.

    Returns:
        The loaded pipeline model.
    """
    spark = get_spark_session()
    if spark._jsc.isLocal():
        return MichelangeloPipelineModel.load(model_path)
    else:
        hdfs_model_dir = f"{HDFS_TMP_MODELS_DIR}/model-{uuid.uuid4()}"
        create_dir_in_hdfs(HDFS_TMP_MODELS_DIR)
        upload_to_hdfs(model_path, hdfs_model_dir)
        return MichelangeloPipelineModel.load(hdfs_model_dir)


def save_spark_pipeline_model(
    model: PipelineModel,
    dest_model_path: Optional[str] = None,
) -> str:
    """
    Save a Spark pipeline model to the given path locally.

    Args:
        model: The pipeline model to save.
        dest_model_path: The local path to save the model.
            If not provided, a temporary directory will be created.

    Returns:
        The path to the saved model.
    """
    if not dest_model_path:
        dest_model_path = tempfile.mkdtemp()
    else:
        os.makedirs(dest_model_path, exist_ok=True)

    spark = get_spark_session()
    if spark._jsc.isLocal():
        model.write().overwrite().save(dest_model_path)
        return dest_model_path
    else:
        hdfs_model_dir = f"{HDFS_TMP_MODELS_DIR}/model-{uuid.uuid4()}"
        model.save(hdfs_model_dir)
        download_from_hdfs(f"{hdfs_model_dir}/*", dest_model_path)
        return dest_model_path
