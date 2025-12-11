class Placeholder:
    """Placeholder for a value that is not yet known."""

    # Used as a placeholder for model name in config.pbtxt file produced by triton packagers
    # The value will be replaced by the actual model name before the file is uploaded to storage in the uploader
    MODEL_NAME = "$MODEL_NAME"
