# Use a base image with Python installed
FROM nvidia/cuda:12.8.0-base-ubuntu22.04

# Install dependencies required for building Python
RUN apt-get update && apt-get install -y \
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
RUN python3.9 -m ensurepip && python3.9 -m pip install --upgrade pip && pip install poetry

# Set working directory
WORKDIR /app

ENV PYTHONPATH=/app

# Copy pyproject.toml and poetry.lock before copying full source
COPY pyproject.toml poetry.lock ./

# Copy the application code
COPY ../.. .

RUN poetry config virtualenvs.create false && poetry install --all-extras --no-interaction --no-ansi && pip install ray[default] numpy==1.23.4 torch==2.0.1 transformers==4.36.2 pandas==1.5.1 pyarrow s3fs==2024.3.0

# Set entrypoint using Poetry
CMD ["bash", "-c", "ray"]