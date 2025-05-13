import tempfile
from typing import Optional
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager._private.packager.common import download_model


class MobileModelPackager:
    def create_model_package(
        self,
        model_path: str,
        dest_model_path: Optional[str] = None,
        model_path_source_type: Optional[str] = StorageType.HDFS,
    ) -> str:
        """
        Create a model package for mobile model

        Args:
            model_path: the path of the raw model
            dest_model_path: the path to save the model package
                If not specified, a temporary directory will be created
            model_path_source_type: the source type of the model path,
                e.g. 'hdfs', 'terrablob', default is 'hdfs'

        Returns:
            The path of the model package
        """
        if not dest_model_path:
            dest_model_path = tempfile.mkdtemp()

        return download_model(
            model_path,
            dest_model_path,
            model_path_source_type,
        )
