import tempfile
from typing import Optional
from pyspark.ml.pipeline import PipelineModel
from pyspark.sql import DataFrame
from uber.ai.michelangelo.sdk.model_manager.schema import ModelSchema
from uber.ai.michelangelo.sdk.model_manager.constants import ModelKind
from uber.ai.michelangelo.sdk.model_manager.utils.model import SparkModelMetadata
from uber.ai.michelangelo.sdk.model_manager._private.packager.common import (
    generate_model_package_folder,
)
from uber.ai.michelangelo.sdk.model_manager._private.packager.spark import (
    generate_model_package_content,
)
from uber.ai.michelangelo.sdk.model_manager._private.schema.spark import (
    create_model_schema,
)


class SparkModelPackager:
    def create_model_package(
        self,
        project_name: str,
        assembled_model: PipelineModel,
        sample_data: DataFrame,
        model_schema: Optional[ModelSchema] = None,
        model_metadata: Optional[SparkModelMetadata] = None,
        model_desc: Optional[str] = "",
        model_kind: Optional[str] = ModelKind.CUSTOM,
        dest_model_path: Optional[str] = None,
    ) -> str:
        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        if not model_schema:
            model_schema = create_model_schema(
                assembled_model,
                sample_data,
            )

        if not model_metadata:
            model_metadata = SparkModelMetadata()

        content = generate_model_package_content(
            project_name,
            assembled_model,
            sample_data,
            model_schema,
            model_metadata,
            model_desc,
            model_kind,
            dest_model_path,
        )

        generate_model_package_folder(
            content,
            dest_model_path,
        )

        return dest_model_path
