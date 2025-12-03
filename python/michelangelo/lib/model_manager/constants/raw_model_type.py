class RawModelType:
    """Raw model types for the raw model package.

    We may move this class to public API in the future.

    Attributes:
        CUSTOM_PYTHON: Custom Python model
        HUGGINGFACE: Huggingface Pipeline
        TORCH: PyTorch model
    """

    CUSTOM_PYTHON = "custom-python"
    HUGGINGFACE = "huggingface"
    TORCH = "torch"
