import tempfile
from typing import Optional, Union
from numpy import ndarray


class CustomTritonPackager:
    def __init__(self, custom_batch_processing: Optional[bool] = False):
        """
        Create a CustomTritonPackager instance
        A packager for custom Triton Python models

        Args:
            custom_batch_processing (Optional):
                If to handle batch manually in the Triton model package.
                Default is False. If set to True, the user is responsible for handling batch in the model class,
                and the model input/output will have an additional batch dimension on top of the existing model schema.
                For example, the schema shape [1] will be converted to [-1, 1].
        """
        self.gen = TritonTemplateRenderer()
        self.custom_batch_processing = custom_batch_processing

