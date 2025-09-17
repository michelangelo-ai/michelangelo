import michelangelo.uniflow.core as uniflow
from examples.bert_cola.data import load_data
from examples.bert_cola.pusher import pusher
from examples.bert_cola.train import train
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC
from michelangelo.uniflow.task.deployment import deploy


@uniflow.workflow()
def train_workflow():
    data_path = "glue"
    data_name = "cola"
    train_data, validation_data, test_data = load_data(
        data_path,
        data_name,
        tokenizer_max_length=128,
    )
    model_uri, train_result, best_checkpoint = train(
        train_data,
        validation_data,
        test_data,
    )
    deployed_model_name = "bert-cola-31"
    model_name = pusher(model_uri, deployed_model_name)
    deploy(namespace="default", name="bert-cola-deployment", model_name=model_name)
    print("result:", train_result)
    print("ok.")


# For Local Run: python3 examples/bert_cola/bert_cola.py
# For Remote Run: python3 examples/bert_cola/bert_cola.py remote-run --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Set the environment variable DATA_SIZE to let the load_data task know how much data to generate.
    ctx.environ["DATA_SIZE"] = "10"

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    # this is example docker image, we don't need to pull it from docker registry
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"
    ctx.environ["MA_API_SERVER"] = "host.docker.internal:14567"
    ctx.environ["MLFLOW_TRACKING_URI"] = (
        "mysql+pymysql://root:root@mysql:3306/mlflow_db"
    )
    # ctx.environ["MLFLOW_TRACKING_URI"] = "mysql+pymysql://root:root@localhost:3306/mlflow_db"

    ctx.run(train_workflow)
