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
    && rm -rf /var/lib/apt/lists/*

# Set Python version (3.11 required for transformers, scikit-learn, etc.)
ENV PYTHON_VERSION=3.11.9

# Download and install Python from source
RUN wget https://www.python.org/ftp/python/$PYTHON_VERSION/Python-$PYTHON_VERSION.tgz && \
    tar xvf Python-$PYTHON_VERSION.tgz && \
    cd Python-$PYTHON_VERSION && \
    ./configure --enable-optimizations && \
    make -j$(nproc) && \
    make altinstall && \
    cd .. && \
    rm -rf Python-$PYTHON_VERSION*

# Ensure python3.11 is the default
RUN ln -sf /usr/local/bin/python3.11 /usr/bin/python3 && \
    ln -sf /usr/local/bin/python3.11 /usr/bin/python

# Install pip and Poetry
RUN python --version
RUN curl -sSL https://install.python-poetry.org | python3 -
ENV PATH="/root/.local/bin:$PATH"
RUN poetry --version


# --------------------------
# 3. Install Spark (manual)
# --------------------------
ENV SPARK_VERSION=3.5.5
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

# Download hadoop-aws and aws-java-sdk-bundle JARs for S3 support
RUN curl -L -o ${SPARK_HOME}/jars/hadoop-aws-3.3.4.jar https://repo1.maven.org/maven2/org/apache/hadoop/hadoop-aws/3.3.4/hadoop-aws-3.3.4.jar && \
    curl -L -o ${SPARK_HOME}/jars/aws-java-sdk-bundle-1.11.1026.jar https://repo1.maven.org/maven2/com/amazonaws/aws-java-sdk-bundle/1.11.1026/aws-java-sdk-bundle-1.11.1026.jar

RUN mkdir -p ${SPARK_HOME}/conf && \
    echo "spark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem" >> ${SPARK_HOME}/conf/spark-defaults.conf && \
    echo "spark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.DefaultAWSCredentialsProviderChain" >> ${SPARK_HOME}/conf/spark-defaults.conf


###
### Install Java
###
# Install OpenJDK 11 (multi-arch compatible and open-source)
RUN apt-get update && apt-get install -y openjdk-11-jdk

# Set JAVA_HOME dynamically based on architecture and update PATH
RUN ARCH=$(dpkg --print-architecture) && \
    echo "Architecture: $ARCH" && \
    export JAVA_HOME=/usr/lib/jvm/java-11-openjdk-${ARCH} && \
    echo "export JAVA_HOME=/usr/lib/jvm/java-11-openjdk-${ARCH}" >> /etc/environment
RUN echo "JAVA_HOME=/usr/lib/jvm/java-11-openjdk-$(dpkg --print-architecture)" >> /etc/environment
ENV JAVA_HOME=/usr/lib/jvm/java-11-openjdk-amd64
ENV PATH="${JAVA_HOME}/bin:${PATH}"

# Set working directory
WORKDIR /app

ENV PYTHONPATH=/app

# Copy the application code
COPY . .

# Install python dependencies
RUN ls -alF ./

# RUN poetry config virtualenvs.create false
# RUN poetry lock
# RUN poetry install -E example
RUN poetry config virtualenvs.create false
RUN rm -f poetry.lock
RUN poetry install -E example --no-root

# Activate Poetry's virtual environment by default
ENV PATH="/app/.venv/bin:$PATH"

COPY examples/entrypoint.sh /opt/entrypoint.sh
RUN chmod +x /opt/entrypoint.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
