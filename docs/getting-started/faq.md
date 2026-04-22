---
sidebar_position: 6
---

# Frequently Asked Questions

Common questions about getting started, data, training, deployment, monitoring, and collaboration.

## Getting Started

**Q: Do I need to learn a new framework to use Michelangelo?**

A: No. If you're using the UI, it's entirely point-and-click. If you're coding, Michelangelo uses familiar tools:
- Python for model code (PyTorch, TensorFlow, scikit-learn, XGBoost all work)
- Ray for distributed computing
- Standard data formats (Parquet, CSV, JSON)
- Decorators (`@uniflow.task`, `@uniflow.workflow`) to integrate your existing code

**Q: Can I use my existing Python ML code?**

A: Yes! Wrap your training functions with `@uniflow.task()` decorator and you're ready to go. Example:
```python
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, head_memory="4Gi"))
def train_model(data_path: str):
    # Your existing training code here
    model = train_my_model(data_path)
    return model
```

**Q: How do I migrate from my current ML stack?**

A: Start small:
1. Pick one model to migrate (not your most critical one)
2. Use Michelangelo's data prep → training → deployment workflow
3. Compare results with your existing pipeline
4. Gradually migrate more models as you gain confidence

## Data & Features

**Q: Where does my training data come from?**

A: Multiple sources:
- Upload CSV/Parquet files directly to the plugged-in storage
- Connect to data warehouses (Snowflake, BigQuery, Redshift)
- Use Spark/Ray for large-scale data processing
- Reference existing datasets in Michelangelo's data catalog

**Q: Can I use feature stores with Michelangelo?**

A: Yes, Michelangelo integrates with feature stores or you can manage features within the platform using the data prep pipelines and inference.

**Q: What data formats are supported?**

A: Parquet (recommended). CSV, JSON, Avro. For custom formats, use Uniflow tasks to handle data loading.

## Training & Deployment

**Q: What compute resources are available?**

A: Michelangelo provides:
- CPU-only instances for lightweight models
- Single-GPU instances (V100, A100) for deep learning
- Multi-GPU clusters for distributed training
- Ray clusters for data-parallel and model-parallel training

**Q: How long does it take to deploy a model?**

A: Deployment time varies:
- Online inference: ~5-10 minutes (container build + rollout)
- Batch predictions: Immediate (scheduled jobs)
- Testing in sandbox: <2 minutes

**Q: Can I do A/B testing?**

A: Yes. Deploy multiple model versions to the same endpoint with traffic splitting. Monitor metrics per variant and gradually shift traffic to the winner.

**Q: What happens if my model training fails?**

A: Uniflow automatically:
- Retries transient failures (network issues, spot instance preemption)
- Preserves logs and intermediate outputs for debugging
- Sends notifications (email, Slack) on terminal state — see [Pipeline Notifications](../user-guides/notifications.md)

## Monitoring & Operations

**Q: How do I monitor model performance in production?**

A: Michelangelo provides:
- **Model Excellence Scores** tracking accuracy, latency, throughput
- **Data drift detection** comparing training vs. production distributions
- **Custom metrics** you define and track
- **Alerts** when metrics degrade beyond thresholds

**Q: Can I roll back a model deployment?**

A: Yes, instantly. Every deployment is versioned. Click "Rollback" in the UI or use the CLI:
```bash
ma deployment -f <deployment spec contains the desired model revision>
```

**Q: How do I debug predictions?**

A: Multiple approaches:
- **Request tracing**: See exact features used for a specific prediction
- **Batch debugging**: Run model on test inputs via UI
- **Local testing**: Pull deployed model and run locally with same inputs

## Cost & Scaling

**Q: How does Michelangelo handle scaling?**

A: Automatically:
- Online inference autoscales based on request volume
- Batch jobs use spot instances for cost savings
- Ray clusters elastically scale workers based on workload

**Q: What if my dataset doesn't fit in memory?**

A: Use Michelangelo's Ray integration for out-of-core processing. Data is streamed from storage (S3, HDFS) and processed in chunks.

## Collaboration & Governance

**Q: How do multiple team members collaborate?**

A: Michelangelo provides:
- **Shared projects** with role-based access (viewer, editor, admin)
- **Model lineage** tracking who trained what and when
- **Version control** for models, datasets, and pipelines
- **Comments and annotations** on experiments and deployments

**Q: Is my data secure?**

A: Yes. Michelangelo enforces:
- Role-based access control (RBAC)
- Encryption at rest and in transit
- Audit logs for all operations
- Compliance with SOC 2, GDPR, HIPAA (depending on deployment)

See the [Compliance Guide](../operator-guides/compliance.md) for configuration steps specific to each framework.

**Q: Can I use Michelangelo for regulated industries (healthcare, finance)?**

A: Yes, with proper configuration. Michelangelo supports:
- Data residency requirements (region-specific storage)
- Audit trails for model decisions
- Explainability tools for model interpretability

---

**Still have questions?** Join the [community forum](https://github.com/michelangelo-ai/michelangelo/discussions) or explore the [user guides](../user-guides/index.md).
