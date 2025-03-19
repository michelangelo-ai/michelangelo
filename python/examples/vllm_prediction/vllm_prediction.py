import michelangelo.uniflow.core as uniflow
from examples.vllm_prediction.predict import predict
from examples.vllm_prediction.data import load_data
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.workflow()
def vllm_prediction_workflow():
    data_slice = ""
    predict_data = load_data(
        path="HuggingFaceH4/MATH-500",
        name=None,
        data_slice="test",
    )
    result = predict(
        predict_data,
        tensor_parallel_size=1,
        worker_instances=1,
        worker_gpu=1,
        batch_size=1,
        model_name="deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B",
        temperature=0.7,
        top_p=0.95,
        max_tokens=100,
    )
    print("result:", result)
    print("ok.")


# For Local Run: python3 examples/bert_cola/bert_cola.py
# For Remote Run: python3 examples/bert_cola/bert_cola.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":

    ctx = uniflow.create_context()

    # Set the environment variable DATA_SIZE to let the load_data task know how much data to generate.
    ctx.environ["DATA_SIZE"] = "10"

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ['PYTORCH_MPS_HIGH_WATERMARK_RATIO'] ='0'
    ctx.environ['MA_NAMESPACE'] ='default'
    # this is example docker image, we don't need to pull it from docker registry
    ctx.environ['IMAGE_PULL_POLICY'] ='Never'
    ctx.run(vllm_prediction_workflow)
