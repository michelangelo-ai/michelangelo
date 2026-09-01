# Michelangelo's default Triton serving image.
#
# The stock nvcr.io/nvidia/tritonserver image's python backend has no ML
# framework deps installed, so any custom python-backend model (produced by
# CustomTritonPackager) that imports torch/transformers at load time fails
# with ModuleNotFoundError. This adds those on top of the stock image so it
# works as the default serving image for Triton-backed InferenceServers --
# see go/components/inferenceserver/backends/triton.go's tritonImage().
#
# The 23.04 image ships Python 3.8, which caps how new a torch/transformers we
# can install (torch 2.4.1 is the last release with 3.8 wheels; transformers
# dropped 3.8 support after the 4.44 series).
#
# A model needing deps/versions outside this image can still override it via
# InferenceServer.spec.initSpec.servingSpec.containerBuildTemplate -- see
# build-example-triton-images.yaml, which builds per-project images from this
# same Dockerfile via EXTRA_PIP_PACKAGES instead of duplicating it per project.
FROM nvcr.io/nvidia/tritonserver:23.04-py3

# Space-separated pip requirement strings (e.g. "torch==2.4.1 transformers==4.44.2").
# Defaults to this image's own baked-in deps so the shared default build
# (build-triton-image.yaml) is unaffected.
ARG EXTRA_PIP_PACKAGES="torch==2.4.1 transformers==4.44.2"

RUN if [ -n "$EXTRA_PIP_PACKAGES" ]; then pip install --no-cache-dir $EXTRA_PIP_PACKAGES; fi
