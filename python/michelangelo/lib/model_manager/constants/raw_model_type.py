"""Raw model type constants."""


class RawModelType:
    """Raw model types for the raw model package.

    Attributes:
        CUSTOM_PYTHON: Custom Python model
        HUGGINGFACE: Huggingface Pipeline
        TORCH: PyTorch model
    """

    CUSTOM_PYTHON = "custom-python"
    HUGGINGFACE = "huggingface"
    TORCH = "torch"
