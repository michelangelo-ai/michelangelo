"""Tests for :mod:`michelangelo.lib.native_transform.torch.id_hash_tokenizer`.

Covers :class:`IDHashTokenizer` mapping of integer IDs to contiguous vocabulary
indices: 2D and 1D inputs, all-out-of-vocabulary inputs, unsorted vocabularies,
duplicate-vocabulary warning, and both TorchScript and ONNX export round-trips.
"""

from __future__ import annotations

import numpy as np
import pytest

# These layers operate on real torch tensors/modules. Skip cleanly if torch is
# unavailable in a lightweight environment.
torch = pytest.importorskip("torch")

from michelangelo.lib.native_transform.torch.id_hash_tokenizer import (  # noqa: E402
    IDHashTokenizer,
)

CUSTOM_VOCAB = [-10, -3, 0, 2, 4, 6]


@pytest.fixture
def tokenizer() -> IDHashTokenizer:
    """Return an ``IDHashTokenizer`` over ``CUSTOM_VOCAB`` in eval mode."""
    module = IDHashTokenizer(vocabulary=CUSTOM_VOCAB)
    module.eval()
    return module


class TestIDHashTokenizer:
    """Mapping, export, and edge-case behavior of :class:`IDHashTokenizer`."""

    def test_2d_lookup(self, tokenizer: IDHashTokenizer) -> None:
        """A 2D batch maps known values by position and unknowns to unk_index."""
        input_batch = torch.tensor(
            [
                [-10, 2, 0, 6],  # All from vocab.
                [0, -3, 5, 4],  # 5 is unknown.
                [10, -10, 2, -100],  # 10 and -100 are unknown.
            ],
            dtype=torch.int32,
        )
        expected = torch.tensor(
            [
                [0, 3, 2, 5],
                [2, 1, 6, 4],
                [6, 0, 3, 6],
            ],
            dtype=torch.int32,
        )
        output = tokenizer(input_batch)
        assert torch.equal(expected, output)

    def test_1d_lookup(self, tokenizer: IDHashTokenizer) -> None:
        """A 1D sequence is mapped element-wise, unknowns to unk_index."""
        input_seq = torch.tensor([4, -3, 999, 6], dtype=torch.long)
        expected = torch.tensor([4, 1, 6, 5], dtype=torch.long)
        output = tokenizer(input_seq)
        assert torch.equal(expected, output)

    def test_all_unknown(self, tokenizer: IDHashTokenizer) -> None:
        """Inputs entirely outside the vocabulary all map to unk_index."""
        input_all_unknown = torch.tensor([[100, 200], [-5, -1000]], dtype=torch.int32)
        expected = torch.tensor([[6, 6], [6, 6]], dtype=torch.int32)
        output = tokenizer(input_all_unknown)
        assert torch.equal(expected, output)

    def test_non_sorted_vocab(self) -> None:
        """An unsorted vocabulary maps by its provided-list position."""
        module = IDHashTokenizer(vocabulary=[-3, -10, 0, 2, 4, 6])
        module.eval()
        input_batch = torch.tensor(
            [
                [-10, 2, 0, 6],
                [0, -3, 5, 4],
                [10, -10, 2, -100],
            ],
            dtype=torch.int32,
        )
        expected = torch.tensor(
            [
                [1, 3, 2, 5],
                [2, 0, 6, 4],
                [6, 1, 3, 6],
            ],
            dtype=torch.int32,
        )
        output = module(input_batch)
        assert torch.equal(expected, output)

    def test_torchscript_roundtrip(self, tokenizer: IDHashTokenizer, tmp_path) -> None:
        """The module scripts, saves, loads, and reproduces eager output.

        native_transform layers must be TorchScript-exportable so the exact
        transform runs at serve time; this guards that contract.
        """
        input_batch = torch.tensor(
            [
                [-10, 2, 0, 6],
                [0, -3, 5, 4],
                [10, -10, 2, -100],
            ],
            dtype=torch.int32,
        )
        eager_output = tokenizer(input_batch)

        scripted = torch.jit.script(tokenizer)
        assert torch.equal(eager_output, scripted(input_batch))

        model_path = tmp_path / "id_hash_tokenizer_scripted.pt"
        scripted.save(str(model_path))
        loaded = torch.jit.load(str(model_path))
        assert torch.equal(eager_output, loaded(input_batch))

    def test_duplicate_vocab_warns_and_dedupes(self) -> None:
        """Duplicate vocabulary values warn and are deduplicated by first index."""
        with pytest.warns(UserWarning, match="Duplicate values"):
            module = IDHashTokenizer(vocabulary=[10, 20, 10])
        assert module.vocabulary == [10, 20]
        assert module.unk_index == 2
        # First occurrence of 10 -> index 0, 20 -> index 1, unknown -> 2.
        output = module(torch.tensor([10, 20, 30], dtype=torch.long))
        assert torch.equal(output, torch.tensor([0, 1, 2], dtype=torch.long))

    def test_rejects_non_integer_vocabulary(self) -> None:
        """A non-integer vocabulary raises ``TypeError``."""
        with pytest.raises(TypeError, match="list of integers"):
            IDHashTokenizer(vocabulary=[1.0, 2.0])  # type: ignore[list-item]

    def test_rejects_empty_vocabulary(self) -> None:
        """An empty vocabulary raises ``ValueError`` at construction time."""
        with pytest.raises(ValueError, match="non-empty"):
            IDHashTokenizer(vocabulary=[])

    def test_rejects_non_integer_input(self, tokenizer: IDHashTokenizer) -> None:
        """A float input tensor raises ``TypeError``."""
        with pytest.raises(TypeError, match="integer type"):
            tokenizer(torch.tensor([1.0, 2.0], dtype=torch.float32))

    def test_onnx_export(self, tokenizer: IDHashTokenizer, tmp_path) -> None:
        """The module exports to ONNX and reproduces eager output via ORT."""
        # ``torch.onnx.export`` needs ``onnx`` and the session needs
        # ``onnxruntime``; skip cleanly if either is missing.
        pytest.importorskip("onnx")
        ort = pytest.importorskip("onnxruntime")
        import torch.onnx

        onnx_model_path = str(tmp_path / "id_hash_tokenizer.onnx")
        dummy_input = torch.tensor([[0, 2, -10]], dtype=torch.int32)
        torch.onnx.export(
            tokenizer,
            dummy_input,
            onnx_model_path,
            export_params=True,
            opset_version=14,
            do_constant_folding=True,
            input_names=["input_ids"],
            output_names=["mapped_ids"],
            dynamic_axes={
                "input_ids": {0: "batch_size", 1: "sequence_length"},
                "mapped_ids": {0: "batch_size", 1: "sequence_length"},
            },
        )

        session = ort.InferenceSession(
            onnx_model_path, providers=["CPUExecutionProvider"]
        )
        test_input = torch.tensor(
            [[-10, 2, 0, 6], [10, -3, 100, 4]], dtype=torch.int32
        ).numpy()
        ort_output = session.run(None, {"input_ids": test_input})[0]
        eager_output = tokenizer(torch.from_numpy(test_input)).numpy()
        assert np.array_equal(eager_output, ort_output)
