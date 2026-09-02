# Michelangelo's default Triton serving image.
#
# The stock nvcr.io/nvidia/tritonserver image's python backend has no ML
# framework deps installed, so any custom python-backend model (produced by
# CustomTritonPackager) that imports torch/transformers at load time fails
# with ModuleNotFoundError. This adds those on top of the stock image so it
# works as the default serving image for Triton-backed InferenceServers --
# see go/components/inferenceserver/backends/triton.go's tritonImage().
#
# torch==2.4.1/transformers==4.44.2 were originally pinned because the prior
# 23.04 base shipped Python 3.8, which capped what was installable. 26.08
# ships a much newer Python, so that cap no longer applies -- these pins are
# no longer forced, just not yet revisited.
#
# A model needing deps/versions outside this image can still override it via
# InferenceServer.spec.initSpec.servingSpec.containerBuildTemplate.
FROM nvcr.io/nvidia/tritonserver:26.08-py3

RUN pip install --no-cache-dir torch==2.4.1 transformers==4.44.2
