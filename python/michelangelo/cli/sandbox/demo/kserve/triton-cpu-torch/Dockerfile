FROM nvcr.io/nvidia/tritonserver:24.09-py3

# CPU-only torch + transformers for real (non-GPU) model serving via Triton's
# python backend.
#
# Two version constraints matter here, both discovered the hard way — see
# ../README.md "Known issues" section for the full story:
#
# 1. Base image must be >= 24.09-py3 (Triton server >= 2.50.0). Triton
#    2.33.0 (23.04-py3) unconditionally aborts the whole process on any
#    CUDA pointer-attribute probe failure during inference, which always
#    happens when no GPU is present. No CUDA_VISIBLE_DEVICES/libcuda.so
#    workaround fixes it — it's baked into that release's compiled
#    libtriton_python.so. 24.09-py3 does not have this bug.
#
# 2. transformers must be pinned to the version the checkpoint was actually
#    saved with. An unpinned `pip install transformers` here previously
#    pulled in transformers 5.13.0, which changed BertModel's internal
#    forward() signature and broke inference with a "not enough values to
#    unpack" error. Update this pin if you retrain against a newer
#    transformers version.
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir transformers==4.46.3
