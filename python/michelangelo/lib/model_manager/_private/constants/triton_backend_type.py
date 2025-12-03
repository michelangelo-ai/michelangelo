class TritonBackendType:
    """Triton Inference Server backend types for deployable model packages.

    These constants correspond to the official backend names supported by Triton.
    They are used in the model configuration (config.pbtxt) to specify which
    backend should be used to load and execute the model.

    Reference: https://github.com/triton-inference-server/server/blob/main/docs/user_guide/model_configuration.md#backend

    Attributes:
        PYTHON: Python backend for custom Python models
        TORCH: PyTorch backend (torchscript models)
        TENSORRT: TensorRT backend for optimized GPU inference
        ONNX: ONNX Runtime backend for ONNX models
    """

    PYTHON = "python"
    TORCH = "pytorch"
    TENSORRT = "tensorrt"
    ONNX = "onnxruntime"
