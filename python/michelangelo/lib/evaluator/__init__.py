"""Evaluator library: metrics reporting for evaluated datasets.

Renders evaluator output DataFrames into an ``EvaluationReport``. Usable on its
own, without a Michelangelo pipeline; the evaluator workflow task is one caller.

Callers should import from the submodules directly, e.g.::

    from michelangelo.lib.evaluator.report_generator import build_combined_report
"""
