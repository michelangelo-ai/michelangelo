# syntax=docker/dockerfile:1
###############################################################################
# $ cd $REPO_ROOT/python
# $ docker build -t <IMAGE_NAME> -f ./examples/bert_cola/Dockerfile .
###############################################################################

# Use a base image with Python installed
FROM nvidia/cuda:12.8.0-base-ubuntu22.04

# Install all system dependencies in a single layer.
# --mount=type=cache persists the apt package download cache across builds.
# We wipe /var/lib/apt/lists/* before updating so that stale package lists
# from a previous build (especially across branches) never cause exit-100
# failures on apt-get install. The download cache in /var/cache/apt is still
# preserved so individual .deb files are not re-fetched on every run.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    rm -rf /var/lib/apt/lists/* && \
    apt-get update && apt-get install -y --no-install-recommends \
    curl \
    software-properties-common \
    vim \
    wget \
    build-essential \
    libssl-dev \
    libreadline-dev \
    libbz2-dev \
    libsqlite3-dev \
    libffi-dev \
    liblzma-dev \
    zlib1g-dev \
    openjdk-17-jdk

# Set Python version
# Spark/PySpark 4.2 requires Python >= 3.10; 3.11 matches the integration-test runner.
ENV PYTHON_VERSION=3.11.17

# Download and install Python from source
RUN wget https://www.python.org/ftp/python/$PYTHON_VERSION/Python-$PYTHON_VERSION.tgz && \
    tar xvf Python-$PYTHON_VERSION.tgz && \
    cd Python-$PYTHON_VERSION && \
    ./configure && \
    make -j$(nproc) && \
    make altinstall && \
    cd .. && \
    rm -rf Python-$PYTHON_VERSION*

# Ensure python3.11 is the default
RUN ln -sf /usr/local/bin/python3.11 /usr/bin/python3 && \
    ln -sf /usr/local/bin/python3.11 /usr/bin/python

# Install pip and Poetry (pinned to 1.8.5)
RUN python --version
RUN curl -sSL https://install.python-poetry.org | POETRY_VERSION=1.8.5 python3 -
ENV PATH="/root/.local/bin:$PATH"
RUN poetry --version


# --------------------------
# 3. Install Spark (manual)
# --------------------------
# Must match the pyspark pin in pyproject.toml (python >= 3.10 branch).
ENV SPARK_VERSION=4.2.0
ENV HADOOP_VERSION=3
ENV SPARK_HOME=/opt/spark

RUN curl -L https://archive.apache.org/dist/spark/spark-${SPARK_VERSION}/spark-${SPARK_VERSION}-bin-hadoop${HADOOP_VERSION}.tgz \
  | tar -xz -C /opt && \
  ln -s /opt/spark-${SPARK_VERSION}-bin-hadoop${HADOOP_VERSION} ${SPARK_HOME}

ENV PATH="${SPARK_HOME}/bin:${SPARK_HOME}/sbin:${PATH}"

# -----------------------------
# 4. Add Hadoop AWS & AWS SDK
# -----------------------------
# Create JAR directory if needed
RUN mkdir -p ${SPARK_HOME}/jars

# Download hadoop-aws and the AWS SDK JARs for S3 support. Versions must match the
# Hadoop client bundled with SPARK_VERSION (Spark 4.2.0 ships hadoop-client-*-3.5.0);
# taken from hadoop-project 3.5.0's POM (aws-java-sdk-v2.version,
# amazon-s3-analyticsaccelerator-s3.version). Hadoop 3.4+ uses AWS SDK v2.
RUN curl -L -o ${SPARK_HOME}/jars/hadoop-aws-3.5.0.jar https://repo1.maven.org/maven2/org/apache/hadoop/hadoop-aws/3.5.0/hadoop-aws-3.5.0.jar && \
    curl -L -o ${SPARK_HOME}/jars/bundle-2.35.4.jar https://repo1.maven.org/maven2/software/amazon/awssdk/bundle/2.35.4/bundle-2.35.4.jar && \
    curl -L -o ${SPARK_HOME}/jars/analyticsaccelerator-s3-1.3.1.jar https://repo1.maven.org/maven2/software/amazon/s3/analyticsaccelerator/analyticsaccelerator-s3/1.3.1/analyticsaccelerator-s3-1.3.1.jar

RUN mkdir -p ${SPARK_HOME}/conf && \
    echo "spark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem" >> ${SPARK_HOME}/conf/spark-defaults.conf && \
    echo "spark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.DefaultAWSCredentialsProviderChain" >> ${SPARK_HOME}/conf/spark-defaults.conf


###
### Install Java
###

# Detect JAVA_HOME at build time so the image works on both arm64 and amd64.
RUN JAVA_HOME=$(dirname $(dirname $(readlink -f $(which java)))) && \
    echo "JAVA_HOME=${JAVA_HOME}" >> /etc/environment && \
    echo "export JAVA_HOME=${JAVA_HOME}" >> /etc/profile.d/java.sh
ARG TARGETARCH=amd64
ENV JAVA_HOME=/usr/lib/jvm/java-17-openjdk-${TARGETARCH}
ENV PATH="${JAVA_HOME}/bin:${PATH}"

# Set working directory
WORKDIR /app

ENV PYTHONPATH=/app

# Copy the application code
COPY . .

# Install python dependencies
RUN ls -alF ./

RUN poetry config virtualenvs.create false
RUN poetry install -E example

# `poetry install -E example` leaves out importlib-resources (a Python < 3.10 requirement of
# matplotlib, pinned in poetry.lock) even though the lock lists it, so `import matplotlib.pyplot`
# fails in the Ray tasks with "No module named 'importlib_resources'". Install the locked
# version explicitly, then fail the build if any other requirement is unmet.
RUN python -m pip install --no-deps "importlib-resources==6.5.2" && python -m pip check

# Activate Poetry's virtual environment by default
ENV PATH="/app/.venv/bin:$PATH"

COPY examples/entrypoint.sh /opt/entrypoint.sh
RUN chmod +x /opt/entrypoint.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
