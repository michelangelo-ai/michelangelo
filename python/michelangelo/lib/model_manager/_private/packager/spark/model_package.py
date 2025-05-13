import os
import tempfile
from pyspark.ml import PipelineModel
from pyspark.sql import DataFrame
from michelangelo.lib.model_manager._private.packager.spark.model_zip import create_model_zip
from michelangelo.lib.model_manager._private.packager.spark.sample_data import create_sample_data_csv
from michelangelo.lib.model_manager._private.packager.spark.model_jar import create_model_jar
from michelangelo.lib.model_manager._private.packager.spark.project_yaml import generate_project_yaml
from michelangelo.lib.model_manager._private.packager.spark.trained_model_yaml import generate_trained_model_yaml
from michelangelo.lib.model_manager._private.packager.spark.model_metadata import generate_model_metadata_content
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_compress
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager.utils.model import SparkModelMetadata


def generate_model_package_content(
    project_name: str,
    assembled_model: PipelineModel,
    sample_data: DataFrame,
    model_schema: ModelSchema,
    model_metadata: SparkModelMetadata,
    model_desc: str,
    model_kind: str,
    dest_model_path: str,
) -> dict:
    """
    Generate the content of a model package.

    Args:
        project_name: The name of the project in MA Studio.
        assembled_model: The assembled spark pipeline model.
        sample_data: A sample data DataFrame.
        model_schema: The model schema.
        model_metadata: The model metadata.
        model_desc: The model description.
        model_kind: The model kind.
        dest_model_path: The destination path for the model package.

    Returns:
        The content of the model package.
    """
    content = None

    with tempfile.TemporaryDirectory() as temp_dir:
        # create model zip
        zip_dir = os.path.join(temp_dir, "model")
        zip_path = create_model_zip(
            assembled_model,
            project_name,
            zip_dir,
        )

        # create sample_data.csv
        os.makedirs(
            os.path.join(dest_model_path, "deploy"),
            exist_ok=True,
        )
        csv_path = os.path.join(
            dest_model_path,
            "deploy",
            "sample_data.csv",
        )
        create_sample_data_csv(sample_data, csv_path)

        # the content for the deploy folder
        deploy_content = {
            **generate_model_metadata_content(model_metadata, model_schema),
            "sample_data.csv": f"file://{csv_path}",
            "trained_model.yaml": generate_trained_model_yaml(
                model_schema,
                project_name,
                model_desc,
                model_kind,
            ),
            "project.yaml": generate_project_yaml(project_name),
        }

        # create model.jar.gz
        model_jar_content = {
            **deploy_content,
            f"{project_name}.zip": f"file://{zip_path}",
        }
        model_jar_path = os.path.join(temp_dir, "model.jar")
        create_model_jar(
            model_jar_content,
            model_jar_path,
        )
        os.makedirs(
            os.path.join(dest_model_path, "deploy_jar"),
            exist_ok=True,
        )
        model_jar_gz_path = os.path.join(
            dest_model_path,
            "deploy_jar",
            "model.jar.gz",
        )
        gzip_compress(model_jar_path, model_jar_gz_path)

        # the finalized content
        content = {
            "deploy": deploy_content,
            "deploy_jar": {
                "model.jar.gz": f"file://{model_jar_gz_path}",
            },
        }

    return content
