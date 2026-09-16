# Generic, project-agnostic serving image for InferenceServer.python_dependencies.
#
# Built on demand by .github/workflows/build-inferenceserver-deps-image.yaml,
# one image per distinct dependency set (tagged by content hash -- see
# go/components/inferenceserver/depsimage), not one per project. Any
# InferenceServer, from any project, that declares the same
# ServingSpec.python_dependencies reuses the same image.
#
# See docker/triton-serving.Dockerfile for the default (no python_dependencies)
# Triton image this is layered on top of.
FROM nvcr.io/nvidia/tritonserver:23.04-py3

# Newline-separated pip requirement strings, passed in by
# build-inferenceserver-deps-image.yaml. No default -- this image only gets
# built when python_dependencies is non-empty.
ARG PYTHON_DEPENDENCIES

RUN pip install --no-cache-dir $PYTHON_DEPENDENCIES
