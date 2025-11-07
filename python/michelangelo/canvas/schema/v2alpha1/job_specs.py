from typing import Optional

from michelangelo.canvas.lib.shared.json_data.field import field, _OneOf, one_of
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData


class ResourceSpec(JSONData):
    # CPU limit in number of CPU cores
    cpu: int = field(default=0, ge=0)

    # Memory limit such as 32G
    memory: str = field(default="")

    # Disk size limit such as 100G
    disk_size: str = field(default="")

    # GPU limit in number of GPUs
    gpu: int = field(default=0, ge=0)

    # GPU sku identifier. Mandatory for cloud workloads
    gpu_sku: str = field(default="")


class PodSpec(JSONData):
    # Resource requirement of the Pod
    resource: ResourceSpec = field(default=ResourceSpec())


class DriverSpec(JSONData):
    # Pod spec for the Spark driver
    pod: PodSpec = field(default=PodSpec())


class ExecutorSpec(JSONData):
    # Pod spec for the Spark executors
    pod: PodSpec = field(default=PodSpec())

    # Number of executor instances
    instances: int = field(default=0, ge=0)


class SparkJobSpec(JSONData):
    # Spark driver specification
    driver: DriverSpec = field(default=DriverSpec())

    # Spark executor specification
    executor: Optional[ExecutorSpec] = field(default=None)

    # Spark configuration
    spark_conf: Optional[dict[str, str]] = field(default=None)

    # jar files
    deps: Optional[dict[str, list[str]]] = field(default=None)


class HeadSpec(JSONData):
    # Pod spec for the Ray head
    pod: PodSpec = field(default=PodSpec())


# Specification for the workers in a Ray job
class WorkerSpec(JSONData):
    # Pod spec for the Ray worker instances
    pod: PodSpec = field(default=PodSpec())

    # Minimum number of worker instances
    min_instances: int = field(default=0, ge=0)

    #  Maximum number of worker instances
    max_instances: int = field(default=0, ge=0)


class RayJobSpec(JSONData):
    # Ray head specification
    head: HeadSpec = field(default=HeadSpec())

    # Ray worker specification
    worker: WorkerSpec = field(default=WorkerSpec())


# Specification of a ML job.
class JobSpecs(JSONData):
    # TODO: support resource pool config
    _one_of_job_specs = one_of(fields=["spark", "ray"], required=False)

    # Spark job
    spark: Optional[SparkJobSpec] = field(default=None)

    # Ray job
    ray: Optional[RayJobSpec] = field(default=None)
