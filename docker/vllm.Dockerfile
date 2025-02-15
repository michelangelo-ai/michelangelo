# Usage:
# Build: docker build -f vllm.Dockerfile -t vllm .
# Run: docker run -it --rm --privileged --shm-size=64gb --runtime=nvidia --gpus all \
#      --network host -v $(pwd):/home/docker/repo -w /home/docker/repo vllm


FROM nvidia/cuda:12.0.0-devel-ubuntu22.04
#FROM nvidia/cuda:12.6.0-devel-ubuntu24.04
#FROM nvidia/cuda:12.8.0-devel-ubuntu24.04
#FROM nvidia/cuda:11.7.1-devel-ubuntu22.04

# Install prerequisite packages
RUN apt-get update && apt-get install -y --no-install-recommends \
  build-essential \
  ca-certificates \
  curl \
  python3 \
  python3-pip \
  python3-venv \
  python3-dev \
  sudo \
  unzip \
  vim \
  wget \
  && rm -rf /var/lib/apt/lists/*

RUN ln -s /usr/bin/python3 /usr/bin/python

# Create and activate a virtual environment
RUN python3 -m venv /home/docker/venv

# Make sure scripts in the venv are prioritized in PATH
ENV PATH="/home/docker/venv/bin:$PATH"

# Upgrade pip and install vllm
RUN python3 -m pip install --upgrade pip \
    && python3 -m pip install --no-cache-dir vllm

