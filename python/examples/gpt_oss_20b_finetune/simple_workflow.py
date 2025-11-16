"""
Simple GPT Fine-tuning Demo (Local Testing Version)
"""

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC

# Import simple functions
from examples.gpt_oss_20b_finetune.simple_train import simple_train_gpt
from examples.gpt_oss_20b_finetune.data import prepare_finetune_dataset
from examples.gpt_oss_20b_finetune.eval import evaluate_gpt_model
from examples.gpt_oss_20b_finetune.package_model import package_gpt_model


@uniflow.workflow()
def simple_gpt_workflow(
    dataset_name="alpaca",
    num_epochs=1,
    sample_size=100,
    model_name="gpt2"
):
    """Simple GPT fine-tuning workflow for testing"""

    # Prepare dataset
    train_dv, val_dv, test_dv = prepare_finetune_dataset(
        dataset_name=dataset_name,
        max_length=512,
        sample_size=sample_size,
        model_name=model_name
    )

    # Train model
    train_result = simple_train_gpt(
        train_dv=train_dv,
        val_dv=val_dv,
        model_name=model_name,
        num_epochs=num_epochs,
        batch_size=1,
        learning_rate=5e-5,
        use_lora=True
    )

    # Evaluate model
    eval_result = evaluate_gpt_model(
        test_dv=test_dv,
        model_path=train_result["model_path"],
        model_name=model_name,
        max_length=512,
        batch_size=1,
        num_samples=20
    )

    # Package model
    package_result = package_gpt_model(
        model_path=train_result["model_path"],
        model_name=model_name,
        package_name=f"{model_name}_finetuned_alpaca"
    )

    # Combine results
    result = {
        "training": train_result,
        "evaluation": eval_result,
        "packaging": package_result
    }

    return result


if __name__ == "__main__":
    print("=" * 60)
    print("Simple GPT Fine-tuning Demo")
    print("=" * 60)

    ctx = uniflow.create_context()

    # Simple environment setup
    ctx.environ["DATA_SIZE"] = "100"
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"
    ctx.environ["RAY_LOG_URL_PREFIX"] = "http://localhost:9091/logs"
    ctx.environ["SPARK_LOG_URL_PREFIX"] = "http://localhost:9091/logs"

    # Run the workflow
    result = ctx.run(
        simple_gpt_workflow,
        dataset_name="alpaca",
        num_epochs=1,
        sample_size=50,
        model_name="gpt2"
    )

    print("=" * 60)
    print("Training completed!")
    print(f"Result: {result}")
    print("=" * 60)