load("@plugin", "pipeline")

def test_run_pipeline():
    return pipeline.run(
        namespace = "default",
        pipeline_name = "test-pipeline",
        input = {
            "test_param": "test_value"
        }
    )

def test_run_pipeline_with_wait():
    return pipeline.run(
        namespace = "default",
        pipeline_name = "test-pipeline",
        input = {"param": "value"},
        wait = True,
        timeout_seconds = 3600
    )

def test_sensor_pipeline():
    return pipeline.sensor(
        namespace = "default",
        pipeline_run_name = "test-pipeline-run-123",
        timeout_seconds = 3600
    )

