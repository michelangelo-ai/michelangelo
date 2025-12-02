from typing import Optional

from michelangelo.canvas.lib.shared.json_data.field import field, one_of
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData


class ResourceSpec(JSONData):
    # CPU limit in number of CPU cores
    cpu: int = field(ge=0)

    # Memory limit such as 32G
    memory: str

    # Disk size limit such as 100G
    disk_size: str

    # GPU limit in number of GPUs
    gpu: int = field(ge=0)

    # GPU sku identifier. Mandatory for cloud workloads
    gpu_sku: str


class PodSpec(JSONData):
    # Resource requirement of the Pod
    resource: ResourceSpec


class DriverSpec(JSONData):
    # Pod spec for the Spark driver
    pod: PodSpec


class ExecutorSpec(JSONData):
    # Pod spec for the Spark executors
    pod: PodSpec

    # Number of executor instances
    instances: int = field(ge=0)


class SparkJobSpec(JSONData):
    # Spark driver specification
    driver: DriverSpec

    # Spark executor specification
    executor: Optional[ExecutorSpec]

    # Spark configuration
    spark_conf: Optional[dict[str, str]]

    # jar files
    deps: Optional[dict[str, list[str]]]


class HeadSpec(JSONData):
    # Pod spec for the Ray head
    pod: PodSpec


# Specification for the workers in a Ray job
class WorkerSpec(JSONData):
    # Pod spec for the Ray worker instances
    pod: PodSpec

    # Minimum number of worker instances
    min_instances: int = field(ge=0)

    #  Maximum number of worker instances
    max_instances: int = field(ge=0)


class RayJobSpec(JSONData):
    # Ray head specification
    head: HeadSpec

    # Ray worker specification
    worker: WorkerSpec


# Specification of a ML job.
class JobSpecs(JSONData):
    # TODO: support resource pool config
    _one_of_job_specs = one_of(fields=["spark", "ray"], required=False)

    # Spark job
    spark: Optional[SparkJobSpec]

    # Ray job
    ray: Optional[RayJobSpec]
