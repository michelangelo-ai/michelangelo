import os
import tempfile
import shutil
from pyspark.ml import PipelineModel
from michelangelo.lib.model_manager.serde.spark import save_spark_pipeline_model


def create_model_zip(
    pipeline_model: PipelineModel,
    zip_name: str,
    dest_dir: str,
) -> str:
    """
    Create a zip file containing a Spark pipeline model.

    Args:
        pipeline_model: A trained Spark pipeline model.
        zip_name: The name of the zip file.
        dest_dir: The directory to save the zip file.

    Returns:
        The path to the saved zip file.
    """
    zip_path = None

    with tempfile.TemporaryDirectory() as temp_dir:
        model_path = save_spark_pipeline_model(
            pipeline_model,
            dest_model_path=temp_dir,
        )

        zip_path = shutil.make_archive(
            os.path.join(dest_dir, zip_name),
            "zip",
            model_path,
        )

    return zip_path
