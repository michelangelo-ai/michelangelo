"""Bounded Buildkite operations and their matching remote Starlark bindings.

Configure the organization and API token on the local process or remote worker,
not in a workflow. Neither credentials nor raw build responses enter results.
"""

import json
import logging
import os
import re
import time
from contextlib import suppress
from dataclasses import dataclass
from http.client import HTTPException
from ipaddress import ip_address
from typing import Optional, TypedDict, TypeVar
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import (
    BaseHandler,
    HTTPRedirectHandler,
    ProxyHandler,
    Request,
    build_opener,
)

from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator

from michelangelo.uniflow.core.decorator import star_plugin

_DEFAULT_API_URL = "https://api.buildkite.com/v2"
_REQUEST_TIMEOUT_SECONDS = 30
_MAX_RESPONSE_BYTES = 8 << 20
_MAX_BUILD_NUMBER = 2**63 - 1
_SUBMISSION_HINT = (
    "; submission outcome may be unknown: "
    "check the pipeline's recent builds before retrying"
)
_PENDING_STATES = frozenset(
    {"creating", "scheduled", "waiting", "running", "failing", "blocked", "canceling"}
)
_FAILED_STATES = frozenset({"failed", "canceled", "skipped", "not_run"})
_logger = logging.getLogger(__name__)


class Build(TypedDict):
    """A compact build snapshot shared with the remote Go/Starlark plugin."""

    id: str
    number: int
    pipeline: str
    state: str
    web_url: str
    branch: str
    commit: str


class _PipelineRequest(BaseModel):
    model_config = ConfigDict(strict=True, hide_input_in_errors=True)

    pipeline: str = Field(pattern=r"^[a-z0-9][a-z0-9_-]*$")


class _TriggerRequest(_PipelineRequest):
    branch: str = Field(default="main", min_length=1)
    commit: str = Field(default="HEAD", min_length=1)
    message: str = ""
    env: Optional[dict[str, str]] = None
    metadata: Optional[dict[str, str]] = Field(
        default=None, serialization_alias="meta_data"
    )

    @field_validator("env")
    @classmethod
    def _valid_env_keys(
        cls, value: Optional[dict[str, str]]
    ) -> Optional[dict[str, str]]:
        for key in value or {}:
            if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key) or key.startswith(
                "BUILDKITE_"
            ):
                raise ValueError(
                    "env keys must match [A-Za-z_][A-Za-z0-9_]* "
                    "and must not start with BUILDKITE_"
                )
        return value


class _BuildRequest(_PipelineRequest):
    build_number: int = Field(gt=0, le=_MAX_BUILD_NUMBER)


class _WaitOptions(BaseModel):
    model_config = ConfigDict(strict=True, hide_input_in_errors=True)

    timeout_seconds: int = Field(default=3600, gt=0, le=86400)
    poll_interval: int = Field(default=30, gt=0, le=3600)
    cancel_on_timeout: bool = True


class _BuildResult(BaseModel):
    model_config = ConfigDict(strict=True, hide_input_in_errors=True)

    id: str = Field(min_length=1)
    number: int = Field(gt=0, le=_MAX_BUILD_NUMBER)
    state: str = Field(min_length=1)
    web_url: str
    branch: str
    commit: str
    blocked: bool = False


@dataclass(frozen=True)
class _Snapshot:
    build: Build
    blocked: bool


class _APIError(RuntimeError):
    def __init__(self, message: str, *, transient: bool) -> None:
        super().__init__(message)
        self.transient = transient


class _NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        """Prevent credential forwarding and mutation replays via redirects."""
        return None


_Model = TypeVar("_Model", bound=BaseModel)


def _validated(function: str, model: type[_Model], **values: object) -> _Model:
    try:
        return model.model_validate(values)
    except ValidationError as error:
        # Keep model names, values, mapping keys, and pydantic URLs private.
        constraints = {
            "pipeline": "must be a slug matching [a-z0-9][a-z0-9_-]*",
            "branch": "must be a non-empty string",
            "commit": "must be a non-empty string",
            "message": "must be a string",
            "env": (
                "must be a dict of strings or None; keys must match "
                "[A-Za-z_][A-Za-z0-9_]* and must not start with BUILDKITE_"
            ),
            "metadata": "must be a dict of strings or None",
            "build_number": f"must be a positive integer at most {_MAX_BUILD_NUMBER}",
            "timeout_seconds": "must be an integer between 1 and 86400",
            "poll_interval": "must be an integer between 1 and 3600",
            "cancel_on_timeout": "must be a bool",
        }
        fields = dict.fromkeys(
            str(item["loc"][0])
            for item in error.errors(include_input=False, include_url=False)
        )
        details = "; ".join(f"{field} {constraints[field]}" for field in fields)
        raise ValueError(f"{function}: {details}") from None


@star_plugin("buildkite.trigger_build")
def trigger_build(
    pipeline: str,
    branch: str = "main",
    commit: str = "HEAD",
    message: str = "",
    env: Optional[dict[str, str]] = None,
    metadata: Optional[dict[str, str]] = None,
) -> Build:
    """Submit one build without waiting or automatically retrying.

    Args:
        pipeline: Pipeline slug in the operator-configured organization.
        branch: Git branch, defaulting to main.
        commit: Git commit, defaulting to HEAD.
        message: Build message.
        env: Optional string environment overrides. Never include secrets.
        metadata: Optional string metadata. Never include secrets.

    Returns:
        A snapshot with id, number, pipeline, state, web_url, branch, and commit.
        Use the pipeline-scoped number, not the UUID id, in subsequent calls.

    Raises:
        ValueError: Arguments or operator configuration are invalid.
        RuntimeError: Submission failed or its acknowledgement is ambiguous.

    Inputs are persisted in workflow history. Configure job secrets on the
    owning pipeline. A lost acknowledgement or workflow reset can still create
    duplicates; inspect Buildkite before resubmitting.
    """
    request = _validated(
        "trigger_build",
        _TriggerRequest,
        pipeline=pipeline,
        branch=branch,
        commit=commit,
        message=message,
        env=env,
        metadata=metadata,
    )
    return _submit(request)


def _submit(request: _TriggerRequest) -> Build:
    return _request(
        "POST",
        request.pipeline,
        payload=request.model_dump(
            by_alias=True, exclude={"pipeline"}, exclude_none=True
        ),
    ).build


@star_plugin("buildkite.get_build")
def get_build(pipeline: str, build_number: int) -> Build:
    """Return a snapshot, including failed builds, without polling.

    Args:
        pipeline: Pipeline slug.
        build_number: Positive, pipeline-scoped build number, not its UUID.

    Returns:
        The compact build snapshot.

    Raises:
        ValueError: Arguments or operator configuration are invalid.
        RuntimeError: Access was denied, the request failed, or the response
            was malformed.
    """
    request = _validated(
        "get_build", _BuildRequest, pipeline=pipeline, build_number=build_number
    )
    return _get_build(request, _REQUEST_TIMEOUT_SECONDS).build


@star_plugin("buildkite.wait_for_build")
def wait_for_build(
    pipeline: str,
    build_number: int,
    timeout_seconds: int = 3600,
    poll_interval: int = 30,
    cancel_on_timeout: bool = True,
) -> Build:
    """Wait for passed; by default, cancel on timeout or local interruption.

    Args:
        pipeline: Pipeline slug.
        build_number: Positive, pipeline-scoped build number.
        timeout_seconds: Wait budget, from 1 to 86400 seconds.
        poll_interval: Poll delay, from 1 to 3600 seconds.
        cancel_on_timeout: Whether timeout or interruption requests cancellation.
            Set False when observing a build you do not own.

    Returns:
        The passed build's compact snapshot.

    Raises:
        ValueError: Arguments or operator configuration are invalid.
        TimeoutError: The wait deadline expired; by default, cleanup requests
            cancellation of the build.
        RuntimeError: The build failed, its state is unknown, or a permanent API
            error occurred.

    Timeout or local interruption requests best-effort cancellation without
    masking the original error. Cleanup has a separate request timeout.
    Socket timeouts are not strict wall-clock bounds; the deadline is also
    checked after each read so a late response cannot report success.
    """
    request = _validated(
        "wait_for_build", _BuildRequest, pipeline=pipeline, build_number=build_number
    )
    options = _validated(
        "wait_for_build",
        _WaitOptions,
        timeout_seconds=timeout_seconds,
        poll_interval=poll_interval,
        cancel_on_timeout=cancel_on_timeout,
    )
    deadline = time.monotonic() + options.timeout_seconds
    last_error: Optional[_APIError] = None

    def expired() -> TimeoutError:
        detail = f"; last poll error: {last_error}" if last_error else ""
        return TimeoutError(
            f"build {pipeline}/{build_number} did not pass within "
            f"{timeout_seconds} seconds{detail}"
        )

    try:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise expired()
            try:
                snapshot = _get_build(request, min(_REQUEST_TIMEOUT_SECONDS, remaining))
            except _APIError as error:
                if not error.transient:
                    raise
                last_error = error
                _logger.warning(
                    "Transient error polling build %s/%s: %s",
                    pipeline,
                    build_number,
                    error,
                )
            else:
                last_error = None
                if time.monotonic() >= deadline:
                    raise expired()
                result = snapshot.build
                if snapshot.blocked:
                    pass
                elif result["state"] == "passed":
                    return result
                elif result["state"] in _FAILED_STATES:
                    raise RuntimeError(
                        f"build {pipeline}/{build_number} ended with state "
                        f"{result['state']}: {result['web_url']}"
                    )
                elif result["state"] not in _PENDING_STATES:
                    raise RuntimeError(
                        f"build {pipeline}/{build_number} has unknown state "
                        f"{result['state']}"
                    )
            remaining = deadline - time.monotonic()
            if remaining > 0:
                time.sleep(min(options.poll_interval, remaining))
    except (TimeoutError, KeyboardInterrupt):
        if options.cancel_on_timeout:
            try:
                cancel_build(pipeline, build_number)
            except Exception:
                _logger.warning(
                    "Could not cancel build %s/%s during cleanup",
                    pipeline,
                    build_number,
                )
        raise


@star_plugin("buildkite.invoke_build")
def invoke_build(
    pipeline: str,
    branch: str = "main",
    commit: str = "HEAD",
    message: str = "",
    env: Optional[dict[str, str]] = None,
    metadata: Optional[dict[str, str]] = None,
    timeout_seconds: int = 3600,
    poll_interval: int = 30,
) -> Build:
    """Submit once, then wait for passed; validate wait options before submitting.

    Args:
        pipeline: Pipeline slug.
        branch: Git branch.
        commit: Git commit.
        message: Build message.
        env: Optional string environment overrides; never include secrets.
        metadata: Optional string metadata; never include secrets.
        timeout_seconds: Wait budget after submission, from 1 to 86400 seconds.
        poll_interval: Poll delay, from 1 to 3600 seconds.

    Returns:
        The passed build's compact snapshot.

    Raises:
        ValueError: Arguments or operator configuration are invalid.
        RuntimeError: Access was denied, submission failed, the build failed,
            or a response was malformed. Inspect recent builds after an
            ambiguous submission before retrying.
        TimeoutError: The wait deadline expired and best-effort cancellation
            was requested for the submitted build.
    """
    _validated(
        "invoke_build",
        _WaitOptions,
        timeout_seconds=timeout_seconds,
        poll_interval=poll_interval,
    )
    request = _validated(
        "invoke_build",
        _TriggerRequest,
        pipeline=pipeline,
        branch=branch,
        commit=commit,
        message=message,
        env=env,
        metadata=metadata,
    )
    build = _submit(request)
    return wait_for_build(pipeline, build["number"], timeout_seconds, poll_interval)


@star_plugin("buildkite.cancel_build")
def cancel_build(pipeline: str, build_number: int) -> Build:
    """Request cancellation once; the returned state may still be canceling.

    Args:
        pipeline: Pipeline slug.
        build_number: Positive, pipeline-scoped build number.

    Returns:
        The compact snapshot, not a guarantee of completed cancellation.

    Raises:
        ValueError: Arguments or operator configuration are invalid.
        RuntimeError: Access was denied, the request failed, or the response
            was malformed.
    """
    request = _validated(
        "cancel_build", _BuildRequest, pipeline=pipeline, build_number=build_number
    )
    snapshot = _request(
        "PUT", request.pipeline, suffix=f"/{request.build_number}/cancel"
    )
    if snapshot.build["number"] != request.build_number:
        raise RuntimeError("Buildkite returned a different build number")
    return snapshot.build


def _get_build(request: _BuildRequest, timeout: float) -> _Snapshot:
    snapshot = _request(
        "GET",
        request.pipeline,
        suffix=f"/{request.build_number}?exclude_jobs=true",
        timeout=timeout,
    )
    if snapshot.build["number"] != request.build_number:
        raise RuntimeError("Buildkite returned a different build number")
    return snapshot


def _endpoint(pipeline: str) -> tuple[str, str, str]:
    organization = os.getenv("BUILDKITE_ORGANIZATION", "")
    if not organization:
        raise ValueError(
            "BUILDKITE_ORGANIZATION is required; "
            "set it on the local process or Go worker"
        )
    # Reuse the strict slug validator without putting configuration in history.
    try:
        _PipelineRequest(pipeline=organization)
    except ValidationError:
        raise ValueError(
            "BUILDKITE_ORGANIZATION must be a pipeline-style slug"
        ) from None
    api_url = os.getenv("BUILDKITE_API_URL") or _DEFAULT_API_URL
    api_url = api_url.rstrip("/")
    try:
        parsed = urlsplit(api_url)
        host = parsed.hostname
        if (
            not host
            or parsed.username is not None
            or parsed.password is not None
            or "?" in api_url
            or "#" in api_url
            or any(ord(character) <= 32 for character in api_url)
        ):
            raise ValueError
        if parsed.scheme != "https":
            loopback = host == "localhost"
            if not loopback:
                with suppress(ValueError):
                    loopback = ip_address(host).is_loopback
            if parsed.scheme != "http" or not loopback:
                raise ValueError
    except ValueError:
        raise ValueError(
            "BUILDKITE_API_URL requires HTTPS (or loopback HTTP), "
            "without credentials, query, or fragment"
        ) from None
    token = os.getenv("BUILDKITE_API_TOKEN", "")
    if api_url == _DEFAULT_API_URL and not token:
        raise ValueError(
            "BUILDKITE_API_TOKEN is required for the Buildkite API; "
            "set it on the local process or Go worker"
        )
    if any(ord(character) < 32 or ord(character) == 127 for character in token):
        raise ValueError("BUILDKITE_API_TOKEN must not contain control characters")
    allowed = os.getenv("BUILDKITE_ALLOWED_PIPELINES", "")
    if not allowed.strip():
        raise _APIError(
            "BUILDKITE_ALLOWED_PIPELINES is required; set comma-separated pipeline "
            "slugs on the local process or Go worker",
            transient=False,
        )
    pipelines = {entry.strip() for entry in allowed.split(",")}
    if any(not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", entry) for entry in pipelines):
        raise ValueError(
            "BUILDKITE_ALLOWED_PIPELINES must contain only comma-separated "
            "pipeline slugs; wildcards are not supported"
        )
    if pipeline not in pipelines:
        raise _APIError(
            f"pipeline {pipeline} is not in BUILDKITE_ALLOWED_PIPELINES",
            transient=False,
        )
    return api_url, organization, token


def _http_error_message(method: str, pipeline: str, status: int) -> str:
    hints = {
        401: (
            "check that BUILDKITE_API_TOKEN is valid on the local process or Go worker"
        ),
        403: "check the token's read_builds/write_builds scopes and pipeline access",
        404: (
            "check BUILDKITE_ORGANIZATION, the pipeline slug, "
            "the build number, and token visibility"
        ),
        422: (
            "Buildkite rejected the request; check the branch, commit, "
            "and build parameters"
        ),
    }
    message = f"Buildkite {method} for {pipeline} failed (HTTP {status})"
    if status in hints:
        message += ": " + hints[status]
    if method == "POST" and (status in (408, 429) or status >= 500):
        message += _SUBMISSION_HINT
    return message


def _response_error(method: str, message: str) -> RuntimeError:
    return RuntimeError(message + (_SUBMISSION_HINT if method == "POST" else ""))


def _request(
    method: str,
    pipeline: str,
    *,
    suffix: str = "",
    payload: Optional[dict[str, object]] = None,
    timeout: float = _REQUEST_TIMEOUT_SECONDS,
) -> _Snapshot:
    api_url, organization, token = _endpoint(pipeline)
    headers = {"Content-Type": "application/json", "Accept": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    request = Request(
        f"{api_url}/organizations/{organization}/pipelines/{pipeline}/builds{suffix}",
        data=None if payload is None else json.dumps(payload).encode("utf-8"),
        headers=headers,
        method=method,
    )
    handlers: list[BaseHandler] = [_NoRedirect()]
    if urlsplit(api_url).scheme == "http":
        # The endpoint validator permits plaintext only for loopback. Never
        # let environment proxy discovery send this request off-host instead.
        handlers.append(ProxyHandler({}))
    try:
        with build_opener(*handlers).open(request, timeout=timeout) as response:
            data = response.read(_MAX_RESPONSE_BYTES + 1)
    except HTTPError as error:
        status = error.code
        error.close()
        raise _APIError(
            _http_error_message(method, pipeline, status),
            transient=status in (408, 429) or status >= 500,
        ) from None
    except (URLError, OSError, HTTPException):
        # Suppress raw errors, which can include private transport details.
        raise _APIError(
            f"Buildkite {method} for {pipeline} could not complete"
            + (_SUBMISSION_HINT if method == "POST" else ""),
            transient=True,
        ) from None
    if len(data) > _MAX_RESPONSE_BYTES:
        raise _response_error(method, "Buildkite response exceeds 8 MiB")
    try:
        raw = json.loads(data)
    except ValueError:
        raise _response_error(method, "Buildkite returned invalid build JSON") from None
    if not isinstance(raw, dict):
        raise _response_error(method, "Buildkite returned no build")
    try:
        result = _BuildResult.model_validate(
            {
                "id": raw.get("id"),
                "number": raw.get("number"),
                "state": raw.get("state"),
                "blocked": False if raw.get("blocked") is None else raw["blocked"],
                **{
                    key: "" if raw.get(key) is None else raw[key]
                    for key in ("web_url", "branch", "commit")
                },
            }
        )
    except ValidationError:
        raise _response_error(
            method, "Buildkite returned an incomplete build"
        ) from None
    return _Snapshot(
        build=Build(
            id=result.id,
            number=result.number,
            pipeline=pipeline,
            state=result.state,
            web_url=result.web_url,
            branch=result.branch,
            commit=result.commit,
        ),
        blocked=result.blocked,
    )
