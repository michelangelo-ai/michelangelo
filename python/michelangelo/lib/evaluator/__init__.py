"""Evaluator library: config-driven metrics, gating, and report rendering.

Computes TorchMetrics metrics over a pandas DataFrame, gates the results
against configured thresholds, and renders them into an ``EvaluationReport``.
Usable on its own, without a Michelangelo pipeline; the evaluator workflow task
is one caller.

Callers should import from the submodules directly::

    from michelangelo.lib.evaluator.evaluator import create_evaluator
    from michelangelo.lib.evaluator.report_generator import build_combined_report

Submodules:
    ``evaluator``: the ``TorchMetricEvaluator`` entry point.
    ``metrics``: metric instantiation, dtype resolution, tensor extraction.
    ``config``: loading an ``EvaluatorConfig`` from a dict or YAML.
    ``chart_data``: ROC/PR curves, regression errors, segment filtering.
    ``sanity``: threshold gating that fails a run on a breached metric.
    ``report`` / ``report_generator``: ``EvaluationReport`` construction.
"""
