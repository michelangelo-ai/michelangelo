# PR #1430 Review Comment Analysis

Reviewed all 20 inline review comments + 5 issue-level comments on
[PR #1430](https://github.com/michelangelo-ai/michelangelo/pull/1430) as of
2026-08-05. For each inline thread: current file state was checked against
the comment, and against internal (`uber-one`) where the comment references
internal behavior, to determine whether it's already resolved by a later
commit, or still open.

## Already resolved (code-verified, no action needed)

These threads have no reply on GitHub, but the underlying code was already
fixed by a later commit in this branch — verified by reading current file
state, not just trusting commit messages.

| # | Reviewer | File | Comment | Resolved by | Verified |
|---|---|---|---|---|---|
| 1 | kenns29 | `schema/data_type.py:62` | "these are spark types, we don't want them in OSS repo" | `2035e4c3` / `735e3035` | `NUMERIC` and Spark-only variants removed; current file has exactly the 11 protobuf-aligned variants. |
| 2 | kenns29 | `schema/data_type.py:44` | "Don't add this in OSS, Numeric is simply Double" | `2035e4c3` | Same as above — `DataType.NUMERIC` no longer exists. |
| 3 | kenns29 | `model_fuser/fused_model.py:1` | "fuser... should not be under assembler" | `e1925a660` et al. | File lives at `lib/shared/utils/model_fuser/fused_model.py`, matching internal's `lib.shared.utils.model_fuser` location. Author (krishpatel9) also explicitly replied "moved to python/michelangelo/lib/shared/utils/model_fuser/". |
| 4 | kenns29 | `model_manager/interface/custom_model.py:85` | "why do we create another custom model interface?" | `735e3035` / `7c53dc73` | Confirmed no duplicate `Model` ABC exists in OSS. `_private/packager/custom_triton/model_interface.py` only *validates against* `custom_model.Model`, mirroring internal's exact `interface/custom_model.py` + `_private/packager/python_triton/model_interface.py` pair (`custom_triton` = OSS's rename of internal's `python_triton`). Diffed both `custom_model.py` files line-by-line — same ABC, same 3 abstract methods, OSS docstrings just more verbose. |
| 5 | kenns29 | `module_finder/dependency_files.py:98` | "corrupted/missed import files... did we test this logic?" | `2035e4c3` | Regression test added (`walk_sys_exit_package` fixture). Author also explicitly replied "Added unit test, and tested with local pipeline run". |
| 6 | kenns29 | `model_fuser/fuse_schema.py:1` | "why is this util in the assembler and not in the fuser?" | moved | Author replied "moved to python/michelangelo/lib/shared/utils/model_fuser/" — confirmed current path. |
| 7 | kenns29 | `torch/assembler.py:45` | "why do we create additional constants instead of using the TritonBackend constant?" | `2035e4c3` | Confirmed: file now imports and uses `TritonBackendType` from `lib.model_manager.constants`; no local `_BACKEND_PYTHON`/`_BACKEND_ONNX` string constants remain. |
| 8 | kenns29 | `tabular_assembler/_private/data/fuse.py:11` | "why is this util in assembler and not in the fuber?" | moved, then moved again | Author replied "now moved to ...model_fuser/" (07-28); this was later reverted back to task-private in `f74491b2` after re-confirming internal's actual current location — internal's `fuse_sample_data` lives at `tabular_assembler/_private/data/fuse.py`, not in the shared fuser. Current placement matches internal exactly. |
| 9 | kenns29 | `schema/assembler.py:17` | "you don't need this constant" | `2035e4c3` / `735e3035` | The `__all__` list this referred to has been removed from the file entirely. |
| 10 | kenns29 | `torch/assembler.py:37` | "these should be private, check the internal impl, and we should not alter the structure too much" (07-28) | `7ee41953` | `_normalize_scalar_shapes`/`_reorder_output_schema` moved out of `torch/assembler.py` into `_private/schema/`, matching internal's private-helper structure. Landed the day after this comment. |
| 11 | kenns29 | `model_fuser/fuse.py:1` | "this file is very outdated... a lot of torchscript and onnx conversion has been decoupled with fuse.py, try pull the latest main and run again" (07-28) | `fe40d783`/`4bb7eca2`/`48684c73` | Ported internal's `ea07350f69c` ONNX-export consolidation the next day: new shared `utils/onnx/torch_onnx.py` + `_private/utils/onnx_utils/onnx_export_helpers.py`, both the fuser and the torch_triton packager rewired onto it. |

**Action for these 11**: none needed in code. Recommend replying on the
threads to close them out, since several have sat unacknowledged for 1-2
weeks and a reviewer re-pinged one of them (`3635718984`, "plz address this
comment") before it was actually fixed.

## Newly fixed in this pass

| # | Reviewer | File | Comment | Fix |
|---|---|---|---|---|
| 12 | kenns29 | `workflow/variables/metadata.py:149` (2026-08-05) | "instead of duplicating these pickling and unpickling logic, we need to have a _private util for BytesIO serialization, refer to the implementation in `uber/ai/michelangelo/sdk/workflow/variables/_private/utils/serialization.py`" | Added `workflow/variables/_private/utils/serialization.py` with generic `retrieve_object`/`save_object` helpers (ported from internal's module, minus the `ModelSchema`/`FeatureSchema`-specific wrappers internal has — OSS has no `feature_manager` module to wrap yet). `ModelMetadata`'s 5 duplicated `seek(0)` / `pickle.loads` / `pickle.dumps` blocks across `schema`, `sample_data`, `transform_spec`, `feature_stats`, `hyperparameters` now all delegate to these two functions. Added `_private/utils/tests/serialization_test.py`. All 634 `workflow/` tests pass; `ruff check`/`format` clean. |

**Also related but not separately actioned**: comment `3669913280` ("these
needs to be handled by the BytesIO too", metadata.py:109, 07-28) and
`3618488066`/`3635718984` (the original BytesIO request, 07-21/07-23) are
all subsumed by the same fix above and by the earlier `2035e4c3` work that
converted `schema`/`sample_data` to lazy `BytesIO`-backed properties in the
first place. All 5 metadata fields (`schema`, `sample_data`,
`transform_spec`, `feature_stats`, `hyperparameters`) are now consistently
`BytesIO`-backed and go through the shared helper.

## Needs your call — architectural, not mechanical

### `scalar_shapes.py` — "we should NEVER modify or rename fields in the sample data or schema" (2026-08-05) — RESOLVED per user decision: went with kenns29

kenns29's comment on `_private/schema/scalar_shapes.py:21`:

> Sample data is to be either inferred from the schema or obtain from the
> previous steps. We always assume the sample data is in the proper shape in
> the assembler, we validate sample data against the schema in the packager,
> but we don't want to modify sample data, we should NEVER modify or rename
> fields in the sample data or schema.

This is a direct objection to what `normalize_scalar_shapes` does: for any
schema item with `shape=[]`, it silently rewrites the schema item to
`shape=[1]` **and** reshapes the corresponding `sample_data` value with
`np.reshape(value, (1,))`. Internal has no equivalent function anywhere —
its packaging path never touches sample data at all; it lets Triton's
`validate_model_schema_item` reject `shape=[]` outright.

Context on why this exists: `sallycr` reported (issue comment, 07-16) that
`ColumnConfig`'s own documented usage (`ColumnConfig("torch.float32")`, no
`shape=`) produced `shape=[]` schema items that the Triton validator
rejected — a real packaging bug for the common scalar-column case. The fix
at the time was two-pronged: (1) make `ColumnConfig.shape` a required field
(`1f46594a`, matching internal, so the documented no-`shape` usage no longer
exists), and (2) keep `normalize_scalar_shapes` as a "defensive backstop" for
the case where a caller still explicitly passes `shape=[]`. There's a
regression test (`ScalarColumnPackagingTest` in `torch/tests/assembler_test
.py`) that locks in silently-reshape-and-succeed behavior for exactly that
case.

kenns29's comment reads as rejecting prong (2) outright, on principle: even
for an explicit `shape=[]`, the assembler should not silently rewrite schema
or reshape sample data — it should let packaging fail loudly, matching
internal, since (2) is exactly the kind of assembler-side sample-data
mutation the comment says should never happen.

**Resolution: went with kenns29.** Removed `normalize_scalar_shapes` and its
call in `torch_assembler` entirely, matching internal's fail-fast behavior
(no assembler-side schema/sample-data rewriting at all). Deleted
`_private/schema/scalar_shapes.py`, its `__init__.py` re-export, and its test
file. Rewrote `ScalarColumnPackagingTest` (`torch/tests/assembler_test.py`)
from asserting silent-reshape-and-succeed to asserting the assembler passes a
`shape=[]` schema item and its `sample_data` value through to the packager
completely unmodified — validating them is the packager's job, not the
assembler's. `ColumnConfig.shape` being required (`1f46594a`) already closed
off the documented no-`shape` path that caused sallycr's original bug, so
this doesn't reopen that regression; it only removes the extra defensive
rewrite for the now-rare explicit-`shape=[]` case, which internal never had
either. 64/64 `tabular_assembler` tests pass; full suite still shows only the
same 10 pre-existing, unrelated failures.

## Issue-level (non-inline) comments — informational, no action

- `sallycr` (07-16): the scalar-shape bug report that led to the
  `ColumnConfig.shape` fix and `normalize_scalar_shapes` — already addressed,
  see above.
- `github-actions[bot]` (07-23, x3) coverage/ruff bot comments on
  `sample_data_fuse_test.py` — that file no longer exists (moved during the
  `fuse_sample_data` relocation); stale, no action possible or needed.
