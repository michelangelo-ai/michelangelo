###############################################################################
# $ cd $REPO_ROOT/python
# $ docker build -t <IMAGE_NAME> -f ./examples/bert_cola/Dockerfile .
###############################################################################

# Use a base image with Python installed
FROM nvidia/cuda:12.8.0-base-ubuntu22.04

# Install dependencies required for building Python
RUN apt-get update && apt-get install -y \
    curl \
    software-properties-common \
    wget \
    build-essential \
    libssl-dev \
    libreadline-dev \
    libbz2-dev \
    libsqlite3-dev \
    libffi-dev \
    liblzma-dev \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# Set Python version
ENV PYTHON_VERSION=3.9.18

# Download and install Python from source
RUN wget https://www.python.org/ftp/python/$PYTHON_VERSION/Python-$PYTHON_VERSION.tgz && \
    tar xvf Python-$PYTHON_VERSION.tgz && \
    cd Python-$PYTHON_VERSION && \
    ./configure --enable-optimizations && \
    make -j$(nproc) && \
    make altinstall && \
    cd .. && \
    rm -rf Python-$PYTHON_VERSION*

# Ensure python3.9 is the default
RUN ln -sf /usr/local/bin/python3.9 /usr/bin/python3 && \
    ln -sf /usr/local/bin/python3.9 /usr/bin/python

# Install pip and Poetry
RUN python --version
RUN python -m ensurepip && python -m pip install --upgrade pip
RUN curl -sSL https://install.python-poetry.org | python3 -
ENV PATH="/root/.local/bin:$PATH"
RUN poetry --version

# Set working directory
WORKDIR /app

ENV PYTHONPATH=/app

# Copy pyproject.toml and poetry.lock before copying full source
COPY pyproject.toml poetry.lock ./

# Copy the application code
COPY . .
# TODO: Change to . . once michelangelo python package is published
# COPY ../.. .

# Install python dependencies
RUN ls -alF ./

RUN poetry config virtualenvs.create false
RUN poetry install -E example

# Activate Poetry's virtual environment by default
ENV PATH="/app/.venv/bin:$PATH"
