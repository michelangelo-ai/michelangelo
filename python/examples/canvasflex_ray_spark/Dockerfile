# syntax=docker/dockerfile:1
###############################################################################
# Slim CPU-only image for the canvasflex_ray_spark example (and other
# non-GPU demos). Skips the CUDA base and the -E example ML stack; installs
# torch from the CPU wheel index (pytorch_lightning is a core dependency).
#
# $ cd $REPO_ROOT/python
# $ docker build -t canvasflex-demo:latest -f ./examples/canvasflex_ray_spark/Dockerfile .
###############################################################################

# python:3.9-slim-bookworm: bookworm ships openjdk-17 (Spark 3.5 supports Java
# 8/11/17; trixie-based tags only carry openjdk-21).
FROM python:3.9-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    wget \
    procps \
    openjdk-17-jre-headless \
    && rm -rf /var/lib/apt/lists/*

# --------------------------
# Spark (matches pyspark version pinned in pyproject.toml)
# --------------------------
ENV SPARK_VERSION=3.5.5
ENV HADOOP_VERSION=3
ENV SPARK_HOME=/opt/spark

RUN curl -L https://archive.apache.org/dist/spark/spark-${SPARK_VERSION}/spark-${SPARK_VERSION}-bin-hadoop${HADOOP_VERSION}.tgz \
  | tar -xz -C /opt && \
  ln -s /opt/spark-${SPARK_VERSION}-bin-hadoop${HADOOP_VERSION} ${SPARK_HOME}

ENV PATH="${SPARK_HOME}/bin:${SPARK_HOME}/sbin:${PATH}"

# Hadoop AWS + AWS SDK JARs for S3 support
RUN curl -L -o ${SPARK_HOME}/jars/hadoop-aws-3.3.4.jar https://repo1.maven.org/maven2/org/apache/hadoop/hadoop-aws/3.3.4/hadoop-aws-3.3.4.jar && \
    curl -L -o ${SPARK_HOME}/jars/aws-java-sdk-bundle-1.11.1026.jar https://repo1.maven.org/maven2/com/amazonaws/aws-java-sdk-bundle/1.11.1026/aws-java-sdk-bundle-1.11.1026.jar

RUN mkdir -p ${SPARK_HOME}/conf && \
    echo "spark.hadoop.fs.s3a.impl=org.apache.hadoop.fs.s3a.S3AFileSystem" >> ${SPARK_HOME}/conf/spark-defaults.conf && \
    echo "spark.hadoop.fs.s3a.aws.credentials.provider=com.amazonaws.auth.DefaultAWSCredentialsProviderChain" >> ${SPARK_HOME}/conf/spark-defaults.conf

RUN JAVA_HOME=$(dirname $(dirname $(readlink -f $(which java)))) && \
    echo "JAVA_HOME=${JAVA_HOME}" >> /etc/environment && \
    echo "export JAVA_HOME=${JAVA_HOME}" >> /etc/profile.d/java.sh
ARG TARGETARCH=amd64
ENV JAVA_HOME=/usr/lib/jvm/java-17-openjdk-${TARGETARCH}
ENV PATH="${JAVA_HOME}/bin:${PATH}"

WORKDIR /app
ENV PYTHONPATH=/app

COPY . .

RUN pip install --no-cache-dir --extra-index-url https://download.pytorch.org/whl/cpu \
    ".[plugin]" pandas "pyarrow>=19.0.0" numpy "s3fs==2024.9.0"

COPY examples/entrypoint.sh /opt/entrypoint.sh
RUN chmod +x /opt/entrypoint.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
