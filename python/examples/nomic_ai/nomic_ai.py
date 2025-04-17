import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC
from examples.nomic_ai.data import load_data
from examples.nomic_ai.train import train


@uniflow.workflow()
def train_workflow():
    model_name = "nomic-ai/nomic-bert-2048"

    train_data, validation_data, test_data = load_data(model_name)

    result = train(
        train_data,
        validation_data,
        test_data,
        model_name,
    )

    print("Training Workflow Result:", result)


if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Set environment variables
    ctx.environ["DATA_SIZE"] = "10"
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"

    ctx.run(train_workflow)
