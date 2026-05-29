# Plan: Use `APIClient` Infrastructure in `GRPCEvalReportSink`

**Date:** 2026-05-28  
**Branch:** `canvasflex/pusher-pr4-eval` (PR #1237)  
**Follow-up:** Separate PR to expose `APIClient.EvaluationReportService` publicly

---

## Context

`GRPCEvalReportSink` currently manages its own raw `grpcio` channel. `APIClient` (in
`python/michelangelo/api/v2/`) provides a `BaseService` base class and `Context`
object that wrap channel management, header injection (`rpc-encoding`, `rpc-service`,
`rpc-caller`), and a retry policy (3 attempts, exponential 0.1 s → 10 s backoff on
INTERNAL / UNAVAILABLE / UNKNOWN). These benefits can be adopted in the sink without
coupling to `APIClient`'s process-wide singleton.

`EvaluationReportService` is **not yet in `APIClient`** — it must be added in a
separate PR after the sink is wired up.

---

## Phase 1 — This PR: wire `GRPCEvalReportSink` to `APIClient` infrastructure

### What changes

**New private service class** inside
`python/michelangelo/workflow/tasks/functions/eval_report_sinks/api.py`:

```python
# Private helper — not exported. Uses APIClient's BaseService pattern
# but with a per-instance channel so each sink can have its own endpoint.
class _EvalReportGRPCService(BaseService):
    def __init__(self, context: Context) -> None:
        super().__init__(context, EvaluationReportServiceStub)

    def create(
        self,
        report: EvaluationReport,
        timeout: int,
        headers: dict | None = None,
    ) -> EvaluationReport:
        from michelangelo.gen.api.v2.evaluation_report_svc_pb2 import (
            CreateEvaluationReportRequest,
        )
        resp = self._stub.CreateEvaluationReport(
            CreateEvaluationReportRequest(evaluation_report=report),
            metadata=self._get_metadata(headers),
            timeout=timeout,
        )
        return resp.evaluation_report
```

**Updated `GRPCEvalReportSink.__init__`**:

```python
def __init__(self, config: GRPCEvalReportSinkConfig) -> None:
    import grpc
    from michelangelo.api.v2.services.base import Context, _DEFAULT_SERVICE_CONFIG, _MAX_MESSAGE_LENGTH
    from michelangelo.gen.api.v2.evaluation_report_svc_pb2_grpc import EvaluationReportServiceStub

    self._channel = (
        grpc.insecure_channel(
            config.endpoint,
            options=[
                ("grpc.service_config", json.dumps(_DEFAULT_SERVICE_CONFIG)),
                ("grpc.max_send_message_length", _MAX_MESSAGE_LENGTH),
                ("grpc.max_receive_message_length", _MAX_MESSAGE_LENGTH),
            ],
        )
        if config.insecure
        else grpc.secure_channel(
            config.endpoint,
            grpc.ssl_channel_credentials(),
            options=[
                ("grpc.service_config", json.dumps(_DEFAULT_SERVICE_CONFIG)),
                ("grpc.max_send_message_length", _MAX_MESSAGE_LENGTH),
                ("grpc.max_receive_message_length", _MAX_MESSAGE_LENGTH),
            ],
        )
    )
    ctx = Context()
    ctx.channel = self._channel
    self._svc = _EvalReportGRPCService(ctx)
    self._config = config
```

**Updated `GRPCEvalReportSink.write`**:

```python
def write(self, report, extra_fields=None):
    if self._config.namespace:
        report.metadata.namespace = self._config.namespace
    try:
        created = self._svc.create(report, timeout=self._config.timeout_seconds)
    except grpc.RpcError as exc:
        raise OSError(
            f"GRPCEvalReportSink: gRPC CreateEvaluationReport failed "
            f"(endpoint={self._config.endpoint!r}, "
            f"code={exc.code()}, details={exc.details()!r})."
        ) from exc
    return EvalReportSinkResult(
        name=created.metadata.name,
        namespace=created.metadata.namespace,
    )
```

### What this gains over raw grpcio

| Feature | Before (raw grpcio) | After (BaseService) |
|---|---|---|
| Retry policy | None | 3 attempts, 0.1 s → 10 s exp backoff on INTERNAL/UNAVAILABLE/UNKNOWN |
| Max message size | Default (4 MB) | 1 GB |
| Standard headers | None | `rpc-encoding: proto`, `rpc-service: ma-apiserver`, `rpc-caller` via `HeaderProvider` |
| Header customisation | Not possible | `ctx.header_provider = CustomProvider()` |
| Channel lifecycle | Managed by sink | Managed by sink (unchanged — `close()` still on `self._channel`) |
| Per-instance endpoint | ✅ | ✅ (preserved — no singleton) |
| TLS support | ✅ `insecure=False` | ✅ (preserved) |

### What does NOT change

- `GRPCEvalReportSinkConfig` interface is identical (`endpoint`, `namespace`, `insecure`, `timeout_seconds`)
- `close()` / `__enter__` / `__exit__` unchanged — `self._channel.close()` still called
- `GRPCEvalReportSink` remains fully self-contained — no `MA_API_SERVER` env var required
- No `APIClient.set_caller()` needed — `DefaultHeaderProvider.caller` is not set (which means `rpc-caller` header is absent unless the user sets it via a custom `HeaderProvider`). If the standard `rpc-caller` header is needed, add a `caller: str | None = None` field to `GRPCEvalReportSinkConfig` and set it on the context's header provider.
- Tests: update mocking from `grpc.insecure_channel` to patch `_EvalReportGRPCService` or `BaseService._stub`

### Schema / config change

None. `GRPCEvalReportSinkConfig` is unchanged.

### Test changes needed

- Replace `patch("grpc.insecure_channel")` / `patch(STUB_PATH)` with
  `patch.object(sink_instance._svc, "create", return_value=mock_report)` — simpler.
- Add a test that verifies the channel is created with the retry service config.
- Existing 37 tests should all pass after mechanical mock update.

---

## Phase 2 — Separate PR: expose `APIClient.EvaluationReportService`

### Scope

Add `EvaluationReportService` as a first-class service on `APIClient`, following the
identical pattern of `PipelineService`, `ModelService`, etc. Purely additive — zero
breaking changes.

### Files to create / change

#### 1. New file: `python/michelangelo/api/v2/services/gen/evaluation_report.py`

```python
from michelangelo.gen.api.v2.evaluation_report_svc_pb2_grpc import EvaluationReportServiceStub
from michelangelo.gen.api.v2.evaluation_report_svc_pb2 import (
    CreateEvaluationReportRequest,
    GetEvaluationReportRequest,
    UpdateEvaluationReportRequest,
    DeleteEvaluationReportRequest,
    DeleteEvaluationReportCollectionRequest,
    ListEvaluationReportRequest,
)
from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1.generated_pb2 import (
    CreateOptions, GetOptions, UpdateOptions, DeleteOptions, ListOptions,
)
from michelangelo.gen.api.list_pb2 import CriterionOperation, ListOptionsExt
from ..base import BaseService, _TIMEOUT_SECONDS


class EvaluationReportService(BaseService):

    def __init__(self, context):
        super().__init__(context, EvaluationReportServiceStub)

    def create_evaluation_report(
        self, evaluation_report, create_options=None, headers=None, timeout=_TIMEOUT_SECONDS
    ):
        """Create an evaluation report.

        Args:
            evaluation_report: EvaluationReport proto to create.
            create_options: Optional CreateOptions.
            headers: Optional request headers dict.
            timeout: Deadline in seconds (default 60).

        Returns:
            The created EvaluationReport proto.

        Example:
            >>> from michelangelo.gen.api.v2.evaluation_report_pb2 import (
            ...     EvaluationReport, EvaluationReportSpec,
            ... )
            >>> from michelangelo.api.v2 import APIClient
            >>> APIClient.set_caller("my-pipeline")
            >>> report = EvaluationReport(spec=EvaluationReportSpec(title="Q1 Eval"))
            >>> report.metadata.namespace = "my-project"
            >>> report.metadata.name = "q1-eval-2026"
            >>> created = APIClient.EvaluationReportService.create_evaluation_report(report)
        """
        req = CreateEvaluationReportRequest(evaluation_report=evaluation_report)
        create_options = self._process_message_or_dict(create_options, CreateOptions)
        req.create_options.CopyFrom(create_options)
        resp = self._stub.CreateEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report

    def get_evaluation_report(self, namespace, name, get_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """Get an evaluation report by namespace and name."""
        req = GetEvaluationReportRequest()
        req.namespace = namespace
        req.name = name
        get_options = self._process_message_or_dict(get_options, GetOptions)
        req.get_options.CopyFrom(get_options)
        resp = self._stub.GetEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_report

    def list_evaluation_report(self, namespace, list_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """List evaluation reports in a namespace."""
        req = ListEvaluationReportRequest()
        req.namespace = namespace
        list_options = self._process_message_or_dict(list_options, ListOptions)
        req.list_options.CopyFrom(list_options)
        resp = self._stub.ListEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
        return resp.evaluation_reports

    def delete_evaluation_report(self, namespace, name, delete_options=None, headers=None, timeout=_TIMEOUT_SECONDS):
        """Delete a single evaluation report."""
        req = DeleteEvaluationReportRequest()
        req.namespace = namespace
        req.name = name
        delete_options = self._process_message_or_dict(delete_options, DeleteOptions)
        req.delete_options.CopyFrom(delete_options)
        self._stub.DeleteEvaluationReport(
            req, metadata=self._get_metadata(headers), timeout=timeout
        )
```

#### 2. Edit `python/michelangelo/api/v2/services/gen/__init__.py`

Add one line to `ServicesGen`:

```python
class ServicesGen(object):
    CachedOutputService = None
    EvaluationReportService = None   # ← add this line
    ModelService = None
    # ... rest unchanged
```

`ServicesGen.init()` discovers it automatically via reflection (camelCase → snake_case →
`evaluation_report` module import). No other wiring needed.

### After Phase 2: optional `GRPCEvalReportSink` update

Once `APIClient.EvaluationReportService` exists, `GRPCEvalReportSink` can optionally
accept `config=None` as a convenience path for callers already inside `APIClient`:

```python
def __init__(self, config: GRPCEvalReportSinkConfig | None = None) -> None:
    if config is None:
        # Convenience path: delegate to APIClient (MA_API_SERVER must be set)
        from michelangelo.api.v2 import APIClient
        self._svc = APIClient.EvaluationReportService
        self._channel = None
        self._config = None
    else:
        # Self-contained path: own channel, any endpoint, TLS-capable
        self._channel = _make_channel(config)
        ctx = Context(); ctx.channel = self._channel
        self._svc = _EvalReportGRPCService(ctx)
        self._config = config
```

This is additive — existing `GRPCEvalReportSink(cfg)` callers are unaffected.

### Phase 2 checklist

- [ ] `python/michelangelo/api/v2/services/gen/evaluation_report.py` (new, ~80 lines)
- [ ] `python/michelangelo/api/v2/services/gen/__init__.py` (+1 line)
- [ ] Unit tests for `EvaluationReportService.create_evaluation_report` (mirror `TestPipelineService`)
- [ ] Update `GRPCEvalReportSink` to accept `config=None` (optional convenience)
- [ ] Docstring on `APIClient` noting the new service
- [ ] PR description: clarify this is an additive convenience — `GRPCEvalReportSink` remains the recommended sink path for custom endpoints

---

## Summary

| | Phase 1 (this PR) | Phase 2 (follow-up PR) |
|---|---|---|
| **Scope** | Wire `GRPCEvalReportSink` to `BaseService` + `Context` | Add `APIClient.EvaluationReportService` |
| **Breaking change** | None | None (purely additive) |
| **Gains** | Retry policy, 1 GB message limit, header framework | Direct SDK entry point for MA-server callers |
| **Singleton coupling** | None — per-instance channel preserved | Only for `config=None` convenience path |
| **Files changed** | `eval_report_sinks/api.py` (sink only) | `services/gen/evaluation_report.py` (new) + `__init__.py` (+1 line) |
| **Test changes** | Mechanical mock update (37 tests) | New service tests (~10 tests) |
