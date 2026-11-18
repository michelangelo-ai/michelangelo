"""Hermetic tests for the public Buildkite API, transport, and compilation."""

import io
import json
import threading
import time
from http.client import IncompleteRead
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import Mock
from urllib.error import HTTPError, URLError

import pytest

from michelangelo.uniflow.core import workflow
from michelangelo.uniflow.core.build import build
from michelangelo.uniflow.core.lib.buildkite import (
    Build,
    cancel_build,
    get_build,
    invoke_build,
    trigger_build,
    wait_for_build,
)
from michelangelo.uniflow.core.lib.buildkite import buildkite as plugin


def snapshot(state: str = "passed", number: int = 17) -> dict:
    """Return a raw build containing fields that must not escape."""
    return {
        "id": f"build-{number}",
        "number": number,
        "state": state,
        "web_url": f"https://buildkite.com/example/smoke-test/builds/{number}",
        "branch": "feature",
        "commit": "sha",
        "env": {"PRIVATE_VALUE": "do-not-return"},
        "jobs": [{"state": state}],
    }


def expected(state: str = "passed", number: int = 17) -> dict:
    """Return the exact seven-field public contract."""
    return {
        key: value
        for key, value in snapshot(state, number).items()
        if key not in ("env", "jobs")
    } | {"pipeline": "smoke-test"}


def response(raw: object) -> io.BytesIO:
    """Build an in-memory, closeable response."""
    return io.BytesIO(json.dumps(raw).encode())


def http_error(status: int) -> HTTPError:
    """Build an HTTP error with a body that must not be surfaced."""
    return HTTPError(
        "https://api.buildkite.com/v2",
        status,
        "private-error-detail",
        {},
        io.BytesIO(b"private-response-body"),
    )


class Clock:
    """Control monotonic time without slowing down polling tests."""

    def __init__(self) -> None:
        """Start at time zero."""
        self.now = 0.0
        self.sleeps: list[float] = []

    def monotonic(self) -> float:
        """Return the current test time."""
        return self.now

    def sleep(self, seconds: float) -> None:
        """Advance the clock and record the requested delay."""
        self.sleeps.append(seconds)
        self.now += seconds


@pytest.fixture(autouse=True)
def configuration(monkeypatch: pytest.MonkeyPatch) -> None:
    """Keep tests independent of developer credentials."""
    monkeypatch.setenv("BUILDKITE_ORGANIZATION", "example")
    monkeypatch.setenv("BUILDKITE_API_TOKEN", "test-token")
    monkeypatch.delenv("BUILDKITE_API_URL", raising=False)


@pytest.fixture
def opener(monkeypatch: pytest.MonkeyPatch) -> Mock:
    """Replace only the network boundary, retaining real request construction."""
    result = Mock()
    monkeypatch.setattr(plugin, "build_opener", Mock(return_value=result))
    return result


@pytest.fixture
def clock(monkeypatch: pytest.MonkeyPatch) -> Clock:
    """Replace the polling clock."""
    result = Clock()
    monkeypatch.setattr(time, "monotonic", result.monotonic)
    monkeypatch.setattr(time, "sleep", result.sleep)
    return result


def test_submission_maps_arguments_and_only_returns_safe_fields(opener: Mock) -> None:
    """Forward per-build options without mutating the caller's dictionaries."""
    opener.open.return_value = response(snapshot("scheduled"))
    env, metadata = {"MODE": "smoke"}, {"release": "bundle"}
    assert trigger_build(
        "smoke-test", "feature", "sha", "smoke", env, metadata
    ) == expected("scheduled")
    request = opener.open.call_args.args[0]
    assert request.method == "POST"
    assert request.full_url == (
        "https://api.buildkite.com/v2/organizations/example/pipelines/smoke-test/builds"
    )
    assert request.get_header("Authorization") == "Bearer test-token"
    assert request.get_header("Accept") == "application/json"
    assert request.get_header("Content-type") == "application/json"
    assert json.loads(request.data) == {
        "branch": "feature",
        "commit": "sha",
        "message": "smoke",
        "env": env,
        "meta_data": metadata,
    }
    assert opener.open.call_args.kwargs == {"timeout": 30}
    assert env == {"MODE": "smoke"}
    assert metadata == {"release": "bundle"}


@pytest.mark.parametrize(
    "options", [{}, {"env": None, "metadata": None}, {"env": {}, "metadata": {}}]
)
def test_defaults_and_nullable_maps(opener: Mock, options: dict) -> None:
    """Omission, explicit None, and empty maps share the same semantics."""
    opener.open.return_value = response(snapshot("scheduled"))
    trigger_build("smoke-test", **options)
    payload = json.loads(opener.open.call_args.args[0].data)
    assert payload["branch"] == "main"
    assert payload["commit"] == "HEAD"
    assert payload["message"] == ""
    assert not payload.get("env")
    assert not payload.get("meta_data")


@pytest.mark.parametrize(
    "function,method,suffix,state",
    [
        (get_build, "GET", "/17", "failed"),
        (cancel_build, "PUT", "/17/cancel", "canceling"),
    ],
)
def test_snapshot_operations(opener: Mock, function, method, suffix, state) -> None:
    """Get and cancel return snapshots, not completion guarantees."""
    opener.open.return_value = response(snapshot(state))
    assert function("smoke-test", 17) == expected(state)
    request = opener.open.call_args.args[0]
    assert request.method == method
    assert request.full_url.endswith("/smoke-test/builds" + suffix)
    assert request.data is None
    opener.open.assert_called_once()


@pytest.mark.parametrize(
    "function,arguments",
    [
        (trigger_build, {"pipeline": ""}),
        (trigger_build, {"pipeline": "../other"}),
        (trigger_build, {"pipeline": "-pipeline"}),
        (trigger_build, {"pipeline": "UPPERCASE"}),
        (trigger_build, {"pipeline": "smoke-test", "branch": ""}),
        (trigger_build, {"pipeline": "smoke-test", "commit": None}),
        (trigger_build, {"pipeline": "smoke-test", "env": {"X": 1}}),
        (trigger_build, {"pipeline": "smoke-test", "env": {"X": None}}),
        (trigger_build, {"pipeline": "smoke-test", "metadata": {1: "X"}}),
        (trigger_build, {"pipeline": "smoke-test", "metadata": ["bad"]}),
        (get_build, {"pipeline": "smoke-test", "build_number": 0}),
        (cancel_build, {"pipeline": "smoke-test", "build_number": -1}),
        (get_build, {"pipeline": "smoke-test", "build_number": True}),
        (get_build, {"pipeline": "smoke-test", "build_number": 2**63}),
        (invoke_build, {"pipeline": "smoke-test", "timeout_seconds": 0}),
        (invoke_build, {"pipeline": "smoke-test", "timeout_seconds": 86401}),
        (invoke_build, {"pipeline": "smoke-test", "poll_interval": 3601}),
        (
            wait_for_build,
            {"pipeline": "smoke-test", "build_number": 17, "poll_interval": -1},
        ),
    ],
)
def test_invalid_arguments_have_no_side_effects(
    opener: Mock, function, arguments: dict
) -> None:
    """Reject malformed inputs before making any request."""
    with pytest.raises(ValueError):
        function(**arguments)
    opener.open.assert_not_called()


def test_wait_polls_pending_states(opener: Mock, clock: Clock) -> None:
    """Only passed completes; all documented nonterminal states keep polling."""
    states = ["creating", "scheduled", "running", "failing", "blocked", "canceling"]
    opener.open.side_effect = [
        response(snapshot(state)) for state in [*states, "passed"]
    ]
    assert wait_for_build("smoke-test", 17, 60, 2) == expected()
    assert clock.sleeps == [2] * len(states)
    assert all(call.args[0].method == "GET" for call in opener.open.call_args_list)


@pytest.mark.parametrize(
    "state", ["failed", "canceled", "skipped", "not_run", "unknown"]
)
def test_terminal_or_unknown_state_fails_without_retry(
    opener: Mock, clock: Clock, state: str
) -> None:
    """Do not retry or cancel an already terminal or unrecognized build."""
    opener.open.return_value = response(snapshot(state))
    with pytest.raises(RuntimeError, match=state):
        wait_for_build("smoke-test", 17)
    opener.open.assert_called_once()
    assert not clock.sleeps


@pytest.mark.parametrize("status", [301, 400, 401, 403, 404, 422])
def test_permanent_http_errors_are_safe_and_not_retried(
    opener: Mock, clock: Clock, status: int
) -> None:
    """Do not expose HTTP error bodies, tokens, or private exception messages."""
    error = http_error(status)
    opener.open.side_effect = error
    with pytest.raises(RuntimeError, match=f"HTTP {status}") as raised:
        wait_for_build("smoke-test", 17)
    assert "private" not in str(raised.value)
    assert "test-token" not in str(raised.value)
    assert raised.value.__suppress_context__
    opener.open.assert_called_once()
    assert error.fp.closed
    assert not clock.sleeps


def test_wait_retries_only_transient_reads(opener: Mock, clock: Clock) -> None:
    """Transport failures, throttling, and server errors may be polled again."""
    opener.open.side_effect = [
        URLError("private-connection-detail"),
        http_error(408),
        http_error(429),
        http_error(503),
        IncompleteRead(b"private-partial-body"),
        response(snapshot()),
    ]
    assert wait_for_build("smoke-test", 17, poll_interval=1) == expected()
    assert clock.sleeps == [1] * 5


@pytest.mark.parametrize("cleanup_status", [200, 403])
def test_wait_timeout_bounds_reads_and_preserves_original_error(
    opener: Mock, clock: Clock, cleanup_status: int
) -> None:
    """Bound read budgets and do not replace a timeout with a cleanup failure."""
    cleanup = (
        response(snapshot("canceling"))
        if cleanup_status == 200
        else http_error(cleanup_status)
    )
    opener.open.side_effect = [
        response(snapshot("running")),
        response(snapshot("running")),
        cleanup,
    ]
    with pytest.raises(TimeoutError, match="did not pass within 5 seconds"):
        wait_for_build("smoke-test", 17, 5, 4)
    assert clock.sleeps == [4, 1]
    assert [call.args[0].method for call in opener.open.call_args_list] == [
        "GET",
        "GET",
        "PUT",
    ]
    assert [call.kwargs["timeout"] for call in opener.open.call_args_list] == [5, 1, 30]


@pytest.mark.parametrize("state", ["passed", "running"])
def test_late_http_success_cannot_outlive_wait_deadline(
    opener: Mock, clock: Clock, state: str
) -> None:
    """Regression: a slow successful read must not bypass the wait deadline."""

    def delayed_response(request, **_kwargs):
        if request.method == "GET":
            clock.now += 6
            return response(snapshot(state))
        return response(snapshot("canceling"))

    opener.open.side_effect = delayed_response
    with pytest.raises(TimeoutError, match="did not pass within 5 seconds"):
        wait_for_build("smoke-test", 17, 5, 1)
    assert [call.args[0].method for call in opener.open.call_args_list] == [
        "GET",
        "PUT",
    ]
    assert not clock.sleeps


def test_local_interruption_requests_cancellation(opener: Mock, clock: Clock) -> None:
    """Preserve local interruption after one cleanup attempt."""
    opener.open.side_effect = [KeyboardInterrupt(), response(snapshot("canceling"))]
    with pytest.raises(KeyboardInterrupt):
        wait_for_build("smoke-test", 17)
    assert [call.args[0].method for call in opener.open.call_args_list] == [
        "GET",
        "PUT",
    ]
    assert not clock.sleeps


def test_invoke_submits_once_then_waits(opener: Mock, clock: Clock) -> None:
    """Invoke composes submission and polling with the same contract."""
    opener.open.side_effect = [
        response(snapshot("scheduled")),
        response(snapshot("running")),
        response(snapshot()),
    ]
    assert (
        invoke_build(
            "smoke-test",
            "feature",
            "sha",
            env={"MODE": "smoke"},
            timeout_seconds=60,
            poll_interval=1,
        )
        == expected()
    )
    assert [call.args[0].method for call in opener.open.call_args_list] == [
        "POST",
        "GET",
        "GET",
    ]
    assert clock.sleeps == [1]


def test_ambiguous_submission_is_never_retried(opener: Mock) -> None:
    """A lost acknowledgement cannot safely be retried by this API."""
    opener.open.side_effect = TimeoutError("private-upstream-detail")
    with pytest.raises(RuntimeError, match="could not complete") as raised:
        invoke_build("smoke-test")
    assert "private" not in str(raised.value)
    opener.open.assert_called_once()


@pytest.mark.parametrize(
    "raw",
    [
        None,
        [],
        {},
        {"id": "uuid", "state": "passed"},
        {"id": "uuid", "number": 0, "state": "passed"},
        {"id": "uuid", "number": True, "state": "passed"},
        {"id": "", "number": 17, "state": "passed"},
        {"id": "uuid", "number": 17, "state": ""},
        {"id": "uuid", "number": 17, "state": "passed", "web_url": False},
    ],
)
def test_malformed_build_is_rejected(opener: Mock, raw: object) -> None:
    """Reject incomplete or mistyped snapshots instead of inventing fields."""
    opener.open.return_value = response(raw)
    with pytest.raises(RuntimeError, match=r"no build|incomplete build"):
        get_build("smoke-test", 17)


def test_missing_optional_fields_normalize_to_empty_strings(opener: Mock) -> None:
    """Absent and null optional strings have the Go implementation's defaults."""
    opener.open.return_value = response(
        {"id": "uuid", "number": 17, "state": "passed", "web_url": None}
    )
    assert get_build("smoke-test", 17) == {
        "id": "uuid",
        "number": 17,
        "pipeline": "smoke-test",
        "state": "passed",
        "web_url": "",
        "branch": "",
        "commit": "",
    }


@pytest.mark.parametrize(
    "body,message",
    [
        (b"not-json", "invalid build JSON"),
        (b"x" * (8 * 1024 * 1024 + 1), "exceeds 8 MiB"),
    ],
)
def test_invalid_or_oversized_json_is_not_retried(
    opener: Mock, body: bytes, message: str
) -> None:
    """Reject unsafe response bodies and release the response resource."""
    stream = io.BytesIO(body)
    opener.open.return_value = stream
    with pytest.raises(RuntimeError, match=message):
        get_build("smoke-test", 17)
    opener.open.assert_called_once()
    assert stream.closed


@pytest.mark.parametrize(
    "variable,value",
    [
        ("BUILDKITE_ORGANIZATION", ""),
        ("BUILDKITE_ORGANIZATION", "../other"),
        ("BUILDKITE_API_TOKEN", ""),
        ("BUILDKITE_API_TOKEN", "private-token\n"),
        ("BUILDKITE_API_URL", "http://example.com/v2"),
        ("BUILDKITE_API_URL", "https://user:private-password@example.com/v2"),
        ("BUILDKITE_API_URL", "https://example.com/v2?private-query"),
        ("BUILDKITE_API_URL", "https://example.com/v2#fragment"),
        ("BUILDKITE_API_URL", "file:///etc/passwd"),
        ("BUILDKITE_API_URL", "https://[bad"),
    ],
)
def test_invalid_configuration_never_makes_a_request(
    opener: Mock, monkeypatch: pytest.MonkeyPatch, variable: str, value: str
) -> None:
    """Configuration errors do not leak supplied secrets or perform mutations."""
    monkeypatch.setenv(variable, value)
    with pytest.raises(ValueError, match=variable) as raised:
        trigger_build("smoke-test")
    assert "private-" not in str(raised.value)
    opener.open.assert_not_called()


@pytest.mark.parametrize(
    "api_url",
    ["http://127.0.0.1:8080/v2/", "http://[::1]:8080/v2", "https://proxy.example/v2"],
)
def test_explicit_proxy_can_supply_its_own_authentication(
    opener: Mock, monkeypatch: pytest.MonkeyPatch, api_url: str
) -> None:
    """Do not require or invent a bearer token for an operator-managed proxy."""
    monkeypatch.setenv("BUILDKITE_API_URL", api_url)
    monkeypatch.delenv("BUILDKITE_API_TOKEN")
    opener.open.return_value = response(snapshot())
    get_build("smoke-test", 17)
    request = opener.open.call_args.args[0]
    assert request.full_url.startswith(api_url.rstrip("/") + "/organizations/")
    assert request.get_header("Authorization") is None


def test_redirect_is_not_followed(monkeypatch: pytest.MonkeyPatch) -> None:
    """Exercise urllib itself: a redirect must not forward the bearer token."""
    paths: list[str] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            paths.append(self.path)
            self.send_response(302)
            self.send_header("Location", "/redirect-target")
            self.end_headers()

        def log_message(self, *_args) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    monkeypatch.setenv("BUILDKITE_API_URL", f"http://127.0.0.1:{server.server_port}/v2")
    try:
        with pytest.raises(RuntimeError, match="HTTP 302"):
            get_build("smoke-test", 17)
        assert paths == ["/v2/organizations/example/pipelines/smoke-test/builds/17"]
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


@workflow()
def compile_probe() -> "list[Build]":
    """Compile every binding without executing any Python plugin body."""
    submitted = trigger_build("smoke-test", env=None, metadata={"smoke": "true"})
    snapshot = get_build("smoke-test", submitted["number"])
    completed = wait_for_build("smoke-test", submitted["number"], 60, 2)
    invoked = invoke_build("smoke-test", env={"MODE": "smoke"}, timeout_seconds=60)
    cancelled = cancel_build("smoke-test", submitted["number"])
    return [snapshot, completed, invoked, cancelled]


def test_compilation_binds_every_operation_without_network(opener: Mock) -> None:
    """All five Python operations must target the existing buildkite namespace."""
    package = build(compile_probe)
    source = package.files[package.main_file].decode("utf-8")
    assert "load('@plugin', __buildkite__='buildkite')" in source
    for operation in [
        "trigger_build",
        "get_build",
        "wait_for_build",
        "invoke_build",
        "cancel_build",
    ]:
        assert f"__buildkite__.{operation}(" in source
    assert "timeout_seconds=60" in source
    assert "env=None" in source
    opener.open.assert_not_called()
