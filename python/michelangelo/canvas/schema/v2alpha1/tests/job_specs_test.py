from google.protobuf import json_format
from typing import Optional
from unittest import TestCase

from michelangelo.canvas.lib.shared.json_data.field import one_of
from michelangelo.canvas.lib.shared.json_data.json_data import JSONData
from michelangelo.canvas.schema.v2alpha1.job_specs import (
    DriverSpec,
    PodSpec,
    JobSpecs,
    RayJobSpec,
    ResourceSpec,
    SparkJobSpec,
)
from michelangelo.gen.api.v2.spark_job_pb2 import SparkJobSpec as UAPISparkJobSpec, DriverSpec as UAPIDriverSpec
from michelangelo.gen.api.v2.pod_pb2 import PodSpec as UAPIPodSpec, ResourceSpec as UAPIResourceSpec


class TestJobSpec(TestCase):

    def test_canvas_job_spec_is_compatible_with_uapi_job_specs(self):
        _cpu = 2
        _memory = "100G"
        _disk_size = "100G"

        spark_job_spec = SparkJobSpec(
            driver=DriverSpec(pod=PodSpec(resource=ResourceSpec(cpu=_cpu, memory=_memory, disk_size=_disk_size))),
            executor=None,
        )
        job_spec = JobSpecs(spark=spark_job_spec)
        json_str = job_spec.model_dump_json(exclude_defaults=False)
        
        # Test with available UAPI SparkJobSpec protobuf
        import json
        job_data = json.loads(json_str)
        spark_json = json.dumps(job_data["spark"])
        
        uapi_spark_job_spec = UAPISparkJobSpec()
        json_format.Parse(spark_json, uapi_spark_job_spec)
        
        self.assertEqual(uapi_spark_job_spec.driver.pod.resource.cpu, _cpu)
        self.assertEqual(uapi_spark_job_spec.driver.pod.resource.memory, _memory)
        self.assertEqual(uapi_spark_job_spec.driver.pod.resource.disk_size, _disk_size)

    def test_canvas_job_spec_compatibility_with_spark_conf(self):
        _cpu = 4
        _memory = "32G"
        _disk_size = "200G"
        _spark_conf = {
            "spark.executor.memory": "8G",
            "spark.dynamicAllocation.enabled": "true",
        }

        spark_job_spec = SparkJobSpec(
            driver=DriverSpec(
                pod=PodSpec(resource=ResourceSpec(cpu=_cpu, memory=_memory, disk_size=_disk_size))
            ),
            executor=None,
            spark_conf=_spark_conf,
        )

        job_spec = JobSpecs(spark=spark_job_spec)
        json_str = job_spec.model_dump_json(exclude_defaults=False)
        
        # Test with available UAPI SparkJobSpec protobuf
        import json
        job_data = json.loads(json_str)
        spark_json = json.dumps(job_data["spark"])
        
        uapi_spark_job_spec = UAPISparkJobSpec()
        json_format.Parse(spark_json, uapi_spark_job_spec)
        
        spark_conf_map = dict(uapi_spark_job_spec.spark_conf)
        self.assertEqual(spark_conf_map["spark.executor.memory"], "8G")
        self.assertEqual(spark_conf_map["spark.dynamicAllocation.enabled"], "true")

    def test_canvas_job_spec_compatibility_with_jars(self):
        _cpu = 2
        _memory = "16G"
        _disk_size = "100G"
        _jars = ["hdfs:///path/to/test1.jar", "hdfs:///path/to/test2.jar"]

        spark_job_spec = SparkJobSpec(
            driver=DriverSpec(
                pod=PodSpec(resource=ResourceSpec(cpu=_cpu, memory=_memory, disk_size=_disk_size))
            ),
            executor=None,
            deps={"jars": _jars},  # Match Python schema
        )

        job_spec = JobSpecs(spark=spark_job_spec)
        json_str = job_spec.model_dump_json(exclude_defaults=False)
        
        # Test with available UAPI SparkJobSpec protobuf
        import json
        job_data = json.loads(json_str)
        spark_json = json.dumps(job_data["spark"])
        
        uapi_spark_job_spec = UAPISparkJobSpec()
        json_format.Parse(spark_json, uapi_spark_job_spec)
        
        jars_list = list(uapi_spark_job_spec.deps.jars)
        self.assertEqual(jars_list, _jars)

    def test_non_compatiable_job_spec(self):
        class NonCompatibleJobSpec(JSONData):
            _one_of_job_spec = one_of(fields=["spark", "ray"], required=False)
            spark: Optional[SparkJobSpec]
            ray: Optional[RayJobSpec]
            # suppose someone add a non-compatible field on Canvas job spec and that field is not in UAPI
            non_compatiable_field: str

        invalid_job_spec = NonCompatibleJobSpec()
        json_str = invalid_job_spec.model_dump_json(exclude_defaults=False)
        
        # Verify that the JSON contains the non-compatible field
        self.assertIn("non_compatiable_field", json_str)
