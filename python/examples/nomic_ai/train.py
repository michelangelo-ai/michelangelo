import torch
import transformers
from transformers import AutoModelForCausalLM, Trainer, TrainingArguments
from ray.data import Dataset
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="3Gi",
        worker_cpu=1,
        worker_memory="3Gi",
        worker_instances=1,
    ),
)
def train(
        train_data: Dataset,
        validation_data: Dataset,
        test_data: Dataset,
) -> dict:
    model_name = "nomic-ai/nomic-embed-text-v1"
    tokenizer = transformers.AutoTokenizer.from_pretrained(model_name)
    model = AutoModelForCausalLM.from_pretrained(model_name)

    # Convert Ray dataset to Hugging Face dataset format
    train_dataset = train_data.to_torch()
    validation_dataset = validation_data.to_torch()

    training_args = TrainingArguments(
        output_dir="./results",
        evaluation_strategy="epoch",
        save_strategy="epoch",
        logging_dir="./logs",
        logging_steps=500,
        per_device_train_batch_size=4,
        per_device_eval_batch_size=4,
        num_train_epochs=1,
        weight_decay=0.01,
        save_total_limit=2,
        fp16=torch.cuda.is_available(),
        push_to_hub=False,
    )

    trainer = Trainer(
        model=model,
        args=training_args,
        train_dataset=train_dataset,
        eval_dataset=validation_dataset,
        tokenizer=tokenizer,
    )

    trainer.train()

    model.save_pretrained("./trained_model")
    tokenizer.save_pretrained("./trained_model")

    return {"status": "Training completed successfully"}
