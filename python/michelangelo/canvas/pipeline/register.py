"""Registration / remote-run bridge for ``pipeline_conf.yaml`` pipelines.

This module makes a YAML-authored pipeline (see
:mod:`michelangelo.canvas.pipeline.config_loader`) registrable and runnable
through the *existing* Uniflow machinery, without duplicating any of it:

- :func:`resolve_workflow_call` turns a loaded :class:`PipelineConfig` into
  the ``(workflow_function, kwargs)`` pair Uniflow expects everywhere else.
  The kwargs values are the **typed**
  :class:`~michelangelo.canvas.schema.v2alpha1.config.TaskConfig` envelope
  objects (not pre-dumped dicts) so the Uniflow JSON encoders attach their
  ``__class__``/``__codec__`` markers when the call is serialized.
- :func:`main` is the CLI equivalent of a ``bert_cola.py``-style entry
  point::

      python -m michelangelo.canvas.pipeline.register pipeline_conf.yaml
          remote-run --image <IMAGE> --storage-url <STORAGE_URL> --yes

  (one command line; wrapped here for readability). Everything after the
  YAML path is handed verbatim to
  :func:`michelangelo.uniflow.core.context.create_context`, so ``local-run``,
  ``remote-run`` and all their flags behave exactly as they do for a plain
  Python-authored pipeline.
- :func:`register_pipeline` is the ``mactl``-oriented equivalent: it feeds
  the resolved workflow call into
  :func:`michelangelo.uniflow.registration.register.register` (tarball via
  ``prepare_uniflow_tar`` + ``uniflow_input.txt`` via
  ``prepare_uniflow_input``).
"""

import inspect
import logging
import sys
from pathlib import Path
from typing import Callable, Optional, Union

from michelangelo.canvas.pipeline.config_loader import (
    PipelineConfig,
    load_pipeline_config,
)
from michelangelo.uniflow.core.context import create_context
from michelangelo.uniflow.core.utils import LOGGING_FORMAT
from michelangelo.uniflow.registration.register import register as uniflow_register

log = logging.getLogger(__name__)

_USAGE = (
    "usage: python -m michelangelo.canvas.pipeline.register "
    "<pipeline_conf.yaml> [local-run|remote-run] [uniflow context args...]"
)


def resolve_workflow_call(pipeline_config: PipelineConfig) -> tuple[Callable, dict]:
    """Resolve a loaded pipeline into ``(workflow_function, call_kwargs)``.

    The workflow function is resolved via
    :meth:`PipelineConfig.resolved_workflow_function` (task names bound into the
    workflow module's globals), and the kwargs are keyed by the workflow's own
    parameter names so they survive keyword-based invocation both in-process and
    after transpilation to Starlark:

    - two-or-more-parameter workflow:
      ``{<param0>: workflow_config, <param1>: task_configs}``
    - single-parameter workflow: ``{<param0>: task_configs}``

    The ``task_configs`` mapping values are the typed ``TaskConfig`` envelopes
    straight from the loader — they must NOT be dumped to plain dicts before
    serialization, otherwise the Uniflow encoders cannot attach the
    ``__class__``/``__codec__`` markers that the Starlark runtime
    (``get_canvas_task_config``) and ``run_task`` decoding rely on.

    Args:
        pipeline_config: A parsed pipeline from :func:`load_pipeline_config`.

    Returns:
        Tuple of the resolved workflow function and the kwargs to call it with.
    """
    fn = pipeline_config.resolved_workflow_function()
    params = list(inspect.signature(inspect.unwrap(fn)).parameters.values())
    workflow_config = pipeline_config.workflow_config

    kwargs = {}
    if len(params) >= 2:
        kwargs[params[0].name] = workflow_config.workflow_config
        kwargs[params[1].name] = workflow_config.task_configs
    else:
        kwargs[params[0].name] = workflow_config.task_configs
    return fn, kwargs


def register_pipeline(
    pipeline_conf_path: Union[str, Path],
    *,
    project: str,
    pipeline: str,
    output_dir: str,
    storage_url: Optional[str] = None,
    output_filename: Optional[str] = None,
    environ: Optional[dict] = None,
) -> str:
    """Register a YAML-authored pipeline through Uniflow's registration flow.

    Thin adapter over :func:`michelangelo.uniflow.registration.register.register`:
    builds/uploads the workflow tarball (``prepare_uniflow_tar``) and writes
    ``uniflow_input.txt`` (``prepare_uniflow_input``) with the resolved workflow
    kwargs, exactly as for a Python-authored pipeline.

    Args:
        pipeline_conf_path: Path to the ``pipeline_conf.yaml`` file.
        project: Michelangelo project name.
        pipeline: Michelangelo pipeline name.
        output_dir: Directory to write registration artifacts for mactl.
        storage_url: Optional tarball storage URL (defaults inside Uniflow).
        output_filename: Optional tar-path output filename.
        environ: Optional environment variables to record in the input file.

    Returns:
        The remote path the workflow tarball was uploaded to.
    """
    fn, kwargs = resolve_workflow_call(load_pipeline_config(pipeline_conf_path))
    return uniflow_register(
        fn=fn,
        project=project,
        pipeline=pipeline,
        output_dir=output_dir,
        storage_url=storage_url,
        output_filename=output_filename,
        environ=environ,
        kwargs=kwargs,
    )


def main(argv: Optional[list[str]] = None):
    """CLI entry point mirroring a ``bert_cola.py``-style runner for YAML pipelines.

    The first argument is the ``pipeline_conf.yaml`` path; everything after it
    is passed to Uniflow's standard execution context. Each of the following
    is a single command line::

        # In-process local run (same semantics as `python bert_cola.py`)
        python -m michelangelo.canvas.pipeline.register pipeline_conf.yaml

        # Remote run against a sandbox
        python -m michelangelo.canvas.pipeline.register pipeline_conf.yaml
            remote-run --image <IMAGE> --storage-url <STORAGE_URL> --yes
    """
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0].startswith("-"):
        raise SystemExit(_USAGE)

    pipeline_conf_path, context_args = argv[0], argv[1:]
    fn, kwargs = resolve_workflow_call(load_pipeline_config(pipeline_conf_path))

    # create_context() parses sys.argv, so hand it everything after the YAML
    # path and let the standard local-run/remote-run flow take over.
    sys.argv = [sys.argv[0], *context_args]
    ctx = create_context()
    ctx.run(fn, **kwargs)


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    main()
