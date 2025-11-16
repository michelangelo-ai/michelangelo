"""
Simple training task for GPT-OSS-20B fine-tuning (local testing version)
"""

import logging
import torch
from transformers import AutoModelForCausalLM, AutoTokenizer, Trainer, TrainingArguments
from peft import LoraConfig, get_peft_model, TaskType
from ray.data import Dataset
from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow
from michelangelo.sdk.workflow.variables import DatasetVariable

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=2,
        head_memory="8Gi",
        worker_cpu=2,
        worker_memory="8Gi",
        worker_instances=1,
        runtime_env={
            "pip": ["transformers", "torch", "peft", "datasets", "accelerate"]
        }
    )
)
def simple_train_gpt(
    train_dv: DatasetVariable,
    val_dv: DatasetVariable,
    model_name="gpt2",
    num_epochs=1,
    batch_size=1,
    learning_rate=5e-5,
    use_lora=True
):
    """
    Simple training function for testing
    """
    log.info(f"Starting simple training with model: {model_name}")

    # Load datasets
    train_dv.load_ray_dataset()
    train_data: Dataset = train_dv.value

    val_dv.load_ray_dataset()
    val_data: Dataset = val_dv.value

    log.info("✅ Datasets loaded")

    # Load tokenizer
    tokenizer = AutoTokenizer.from_pretrained(model_name)
    if tokenizer.pad_token is None:
        tokenizer.pad_token = tokenizer.eos_token

    # Load model
    model = AutoModelForCausalLM.from_pretrained(
        model_name,
        torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32,
        device_map="auto" if torch.cuda.is_available() else None
    )

    # Setup LoRA if enabled
    if use_lora:
        lora_config = LoraConfig(
            r=16,
            lora_alpha=32,
            lora_dropout=0.1,
            target_modules=["c_attn", "c_proj"],
            bias="none",
            task_type=TaskType.CAUSAL_LM
        )
        model = get_peft_model(model, lora_config)
        model.print_trainable_parameters()

    log.info("✅ Model loaded")

    # Convert Ray datasets to simple format
    train_df = train_data.to_pandas()
    val_df = val_data.to_pandas()

    # Create simple dataset class
    class SimpleDataset(torch.utils.data.Dataset):
        def __init__(self, dataframe):
            self.data = dataframe

        def __len__(self):
            return len(self.data)

        def __getitem__(self, idx):
            item = self.data.iloc[idx]
            return {
                "input_ids": torch.tensor(item["input_ids"], dtype=torch.long),
                "attention_mask": torch.tensor(item["attention_mask"], dtype=torch.long),
                "labels": torch.tensor(item["labels"], dtype=torch.long)
            }

    train_dataset = SimpleDataset(train_df)
    val_dataset = SimpleDataset(val_df)

    # Training arguments
    training_args = TrainingArguments(
        output_dir="/tmp/simple_train",
        num_train_epochs=num_epochs,
        per_device_train_batch_size=batch_size,
        per_device_eval_batch_size=batch_size,
        learning_rate=learning_rate,
        logging_steps=10,
        eval_strategy="steps",  # Updated API name
        eval_steps=100,
        save_steps=500,
        remove_unused_columns=False,
        report_to=None
    )

    # Data collator
    from transformers import DataCollatorForLanguageModeling
    data_collator = DataCollatorForLanguageModeling(
        tokenizer=tokenizer,
        mlm=False
    )

    # Create trainer
    trainer = Trainer(
        model=model,
        args=training_args,
        train_dataset=train_dataset,
        eval_dataset=val_dataset,
        data_collator=data_collator,
        tokenizer=tokenizer
    )

    # Train
    log.info("Starting training...")
    train_result = trainer.train()

    # Save
    output_dir = "/tmp/simple_model"
    trainer.save_model(output_dir)
    tokenizer.save_pretrained(output_dir)

    log.info("✅ Training completed")

    return {
        "model_path": output_dir,
        "train_loss": train_result.training_loss,
        "training_type": "simple_local"
    }