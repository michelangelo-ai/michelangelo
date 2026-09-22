# Michelangelo's default Triton serving image.
#
# The stock nvcr.io/nvidia/tritonserver image ships a python backend without
# ML framework dependencies, so a python-backend model that imports torch or
# transformers at load time fails with ModuleNotFoundError. This installs them
# on top of the stock image.
#
# .github/workflows/build-triton-image.yaml publishes this to GHCR for amd64
# and arm64, tagged with both the base version and the commit it was built
# from. Editing this file publishes new tags; nothing repoints at them
# automatically.
FROM nvcr.io/nvidia/tritonserver:25.01-py3

RUN pip install --no-cache-dir torch==2.4.1 transformers==4.44.2
