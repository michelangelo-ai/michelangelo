# Michelangelo's default Triton serving image.
#
# The stock nvcr.io/nvidia/tritonserver image ships a python backend without
# ML framework dependencies, so a python-backend model that imports torch or
# transformers at load time fails with ModuleNotFoundError. This installs them
# on top of the stock image.
#
# .github/workflows/build-triton-image.yaml publishes this to GHCR for amd64
# and arm64. defaultTritonImage in go/components/inferenceserver/backends/
# triton.go pins one of those builds, so a change here needs that tag bumped
# to the resulting image.
#
# An InferenceServer needing other versions overrides the image through
# spec.initSpec.servingSpec.image.
FROM nvcr.io/nvidia/tritonserver:25.01-py3

RUN pip install --no-cache-dir torch==2.4.1 transformers==4.44.2
