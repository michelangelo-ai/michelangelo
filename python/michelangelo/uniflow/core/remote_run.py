import argparse
import base64
import json
import logging
import os
import random
import re
import string
import subprocess
import sys
from dataclasses import dataclass, field
from typing import Callable, Optional

from michelangelo.uniflow.core.codec import encoder
from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.core.utils import dot_path

log = logging.getLogger(__name__)

DEFAULT_EXECUTION_TIMEOUT_SECONDS = (
    60 * 60 * 24 * 365 * 10
)  # 3650 days, practically no timeout

_RUN_ID_SEARCH_RE = re.compile(
    r"run[ _-]?id[:= ]{1,2}([0-9a-f-]+)", flags=re.IGNORECASE
)  # Run ID is a UUID


@dataclass
class RemoteRun:
    fn: Callable
    image: str
    storage_url: str
    metadata_storage_url: Optional[str] = None
    environ: dict[str, str] = field(default_factory=dict)
    args: tuple = field(default_factory=tuple)
    kwargs: dict = field(default_factory=dict)
    cron: Optional[str] = None
    retry_expiration: Optional[int] = None
    retry_attempts: Optional[int] = None
    retry_interval: Optional[int] = None
    retry_backoff: Optional[int] = None
    retry_max_interval: Optional[int] = None
    execution_timeout_seconds: int = DEFAULT_EXECUTION_TIMEOUT_SECONDS
    yes: bool = False

    def run(self):
        tarball = build(self.fn).to_tarball_bytes()
        log.info("tarball: total bytes: %d", len(tarball))

        tarball = base64.standard_b64encode(tarball)
        log.info("tarball_b64: total bytes: %d", len(tarball))

        tarball = tarball.decode("utf-8")

        rand_str = "".join(random.choices(string.ascii_lowercase + string.digits, k=5))
        workflow_id = f"{dot_path(self.fn)[-120:]}.{rand_str}"

        environ = self.environ.copy()
        environ["UF_TASK_IMAGE"] = self.image
        environ["UF_STORAGE_URL"] = self.storage_url
        if self.metadata_storage_url:
            environ["UF_METADATA_STORAGE_URL"] = self.metadata_storage_url

        for k, v in environ.items():
            log.info("environ: %s: %s", k, v)

        for i, a in enumerate(self.args):
            log.info("arg: %d: %r", i, a)

        for k, v in self.kwargs.items():
            log.info("arg: %s: %r", k, v)

        input_ = []
        for el in [tarball, "", "", self.args, list(self.kwargs.items()), environ]:
            el = json.dumps(el, separators=(",", ":"), default=encoder.default)
            input_ += [el, "\n"]

        input_ = "".join(input_)

        log.debug("input: %s", input_)
        log.info("input: total bytes: %d", len(input_))

        cmd = ["cadence"]

        if cadence_env := os.environ.get("UFC_CADENCE_ENV"):
            cmd += ["--env", cadence_env]

        if cadence_proxy_region := os.environ.get("UFC_CADENCE_PROXY_REGION"):
            cmd += ["--proxy_region", cadence_proxy_region]

        cmd += [
            "--domain",
            os.environ.get("UFC_CADENCE_DOMAIN", "default"),
            "workflow",
            "start",
            "--tasklist",
            os.environ.get("UFC_CADENCE_TASK_LIST", "default"),
            "--workflow_type",
            os.environ.get(
                "UFC_CADENCE_WORKFLOW_TYPE",
                "github.com/cadence-workflow/starlark-worker/cadstar.(*Service).Run",
            ),
            "--execution_timeout",
            str(self.execution_timeout_seconds),
            "--workflow_id",
            workflow_id,
        ]

        if self.cron:
            cmd += ["--cron", self.cron]
        if self.retry_expiration:
            cmd += ["--retry_expiration", self.retry_expiration]
        if self.retry_attempts:
            cmd += ["--retry_attempts", self.retry_attempts]
        if self.retry_interval:
            cmd += ["--retry_interval", self.retry_interval]
        if self.retry_backoff:
            cmd += ["--retry_backoff", self.retry_backoff]
        if self.retry_max_interval:
            cmd += ["--retry_max_interval", self.retry_max_interval]

        log.info("%r", cmd)
        cmd += ["--input", input_]

        log.debug("[+] %r", cmd)

        if not self.yes:
            print()
            a = None
            while a not in ("y", "n", ""):
                a = input("Run the workflow in the Remote Mode? [Y/n]").lower()
            if a == "n":
                raise RuntimeError("User interrupted the Remote Run submission")

        try:
            stdout = subprocess.check_output(cmd, text=True)
        except KeyboardInterrupt:
            sys.exit(130)

        print(stdout)

        dashboard_url = os.environ.get("UFC_CADENCE_DASHBOARD_URL", "")

        if not dashboard_url:
            return

        # Extract Run ID
        run_id = _RUN_ID_SEARCH_RE.findall(stdout)
        if len(run_id) != 1:
            # Failed to extract RunID from the Cadence stdout.
            print("RunID: %r", run_id)
            print("Dashboard:", dashboard_url)
            return

        print(
            "Dashboard:", f"{dashboard_url}/workflows/{workflow_id}/{run_id[0]}/summary"
        )
