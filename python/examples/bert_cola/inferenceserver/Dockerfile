# Custom Triton serving image for the bert_cola example.
#
# The stock nvcr.io/nvidia/tritonserver image's python backend has no ML
# framework deps installed, so a custom python-backend model (produced by
# CustomTritonPackager -- see examples/bert_cola/assembler.py) that imports
# torch/transformers at load time fails with ModuleNotFoundError. This adds
# just those two packages on top of the stock image; nothing else changes.
#
# The 23.04 image ships Python 3.8, which caps how new a torch/transformers we
# can install (torch 2.4.1 is the last release with 3.8 wheels; transformers
# dropped 3.8 support after the 4.44 series).
FROM nvcr.io/nvidia/tritonserver:23.04-py3

RUN pip install --no-cache-dir torch==2.4.1 transformers==4.44.2
