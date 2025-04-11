import unittest
import tempfile

import torch
from michelangelo.sdk.core.models.tte.text_encoder import Me5Encoder, NomicEncoder, QWenEncoder, SimpleReductionLayer, TextEncoder

import os

dir_path = os.path.dirname(os.path.realpath(__file__))


class TestSimpleReduction(unittest.TestCase):
    def setUp(self):
        self.model = SimpleReductionLayer(input_dim=1536, output_dim=128)

    def test_forward(self):
        embeddings = torch.randn(2, 1536)
        new_embeddings = self.model(embeddings)
        assert new_embeddings.shape == (2, 128)


class TransferTextEncoderTest(unittest.TestCase):
    def test_text_encoder(self):
        # test no dimension reduction
        tmp_model_dir = tempfile.TemporaryDirectory().name
        pretrained_model_dir = os.path.join(dir_path, "unit_test_model_files")

        encoder = TextEncoder(pretrained_model_dir, 0)
        encoder.freeze_llm_layers(0)
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 32))
        encoder = TextEncoder(pretrained_model_dir, pooling_strategy="avg")
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 32))
        encoder = TextEncoder(pretrained_model_dir, pooling_strategy="last_token")
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 32))
        encoder.save_model(tmp_model_dir)

        with self.assertRaises(NotImplementedError):
            encoder = TextEncoder(pretrained_model_dir, pooling_strategy="wrong_pool")
            encoder.encode("Hello World")

        # test dimension reduction
        encoder = TextEncoder(pretrained_model_dir, reshape_layer_input_dim=32, reshape_layer_output_dim=8)
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        emb = encoder.encode("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        encoder.save_model(tmp_model_dir)

        # test pre-trained dimension reduction
        encoder = TextEncoder(pretrained_model_dir, pretrained_reduction_script_file="script_reduction_model.pt")
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        emb = encoder.encode("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        encoder.save_model(tmp_model_dir)

        encoder_reloaded = TextEncoder(tmp_model_dir, pretrained_reduction_script_file="script_reduction_model.pt")
        emb_reloaded = encoder_reloaded.forward("Hello World")
        assert torch.allclose(emb_reloaded, emb)

        encoder = TextEncoder(pretrained_model_dir, reduction_layer_args={"input_dim": 32, "output_dim": 8})
        emb = encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        emb = encoder.encode("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        encoder.save_model(tmp_model_dir)

    def test_derived_text_encoder(self):
        pretrained_model_dir = os.path.join(dir_path, "unit_test_model_files")
        qwen_encoder = QWenEncoder(pretrained_model_dir, reshape_layer_input_dim=32, reshape_layer_output_dim=8)
        qwen_encoder.freeze_llm_layers(-1)
        qwen_encoder.freeze_llm_layers(0)
        with self.assertRaises(ValueError):
            qwen_encoder.freeze_llm_layers(-10)
        with self.assertRaises(AttributeError):
            qwen_encoder.freeze_llm_layers(1)

        nomic_encoder = NomicEncoder(pretrained_model_dir, reshape_layer_input_dim=32, reshape_layer_output_dim=8)
        nomic_encoder.freeze_llm_layers(-1)
        nomic_encoder.freeze_llm_layers(0)
        emb = nomic_encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        emb = nomic_encoder.encode("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        with self.assertRaises(ValueError):
            nomic_encoder.freeze_llm_layers(-10)
        with self.assertRaises(AttributeError):
            nomic_encoder.freeze_llm_layers(1)

        me5_encoder = Me5Encoder(pretrained_model_dir, reshape_layer_input_dim=32, reshape_layer_output_dim=8)
        me5_encoder.freeze_llm_layers(-1)
        me5_encoder.freeze_llm_layers(0)
        emb = me5_encoder.forward("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        emb = me5_encoder.encode("Hello World")
        self.assertEqual(emb.shape, (1, 8))
        with self.assertRaises(ValueError):
            me5_encoder.freeze_llm_layers(-10)
        me5_encoder.freeze_llm_layers(1)
        with self.assertRaises(ValueError):
            me5_encoder.freeze_llm_layers(10)

    def test_last_token_pool(self):
        # hidden states is in shape (B, L, D) where B is the batch size, L is the sequence length and D is the embedding dimension
        last_hidden_states = torch.randn(2, 3, 384)
        # 0 in mast token means pad left
        attention_mask = torch.tensor([[0, 1, 1], [1, 1, 1]])
        results = TextEncoder.last_token_pool(last_hidden_states, attention_mask)
        expected = torch.cat([last_hidden_states[0, -1, :].unsqueeze(0), last_hidden_states[1, -1, :].unsqueeze(0)], dim=0)
        assert results.shape == (2, 384)
        assert torch.allclose(results, expected)
        attention_mask = torch.tensor([[1, 1, 0], [1, 0, 0]])
        results = TextEncoder.last_token_pool(last_hidden_states, attention_mask)
        expected = torch.cat([last_hidden_states[0, 1, :].unsqueeze(0), last_hidden_states[1, 0, :].unsqueeze(0)], dim=0)
        assert torch.allclose(results, expected)

    def test_avg_token_pool(self):
        last_hidden_states = torch.randn(2, 3, 384)
        # 0 in mast token means pad left
        attention_mask = torch.tensor([[0, 1, 1], [1, 1, 1]])
        results = TextEncoder.average_pool(last_hidden_states, attention_mask)
        expected = torch.cat(
            [
                (0.5 * last_hidden_states[0, -1, :] + 0.5 * last_hidden_states[0, -2, :]).unsqueeze(0),
                torch.mean(last_hidden_states[1, :, :], dim=0).unsqueeze(0),
            ],
            dim=0,
        )
        assert torch.allclose(results, expected)

    def test_mean_token_pool(self):
        last_hidden_states = torch.randn(2, 3, 384)
        # 0 in mast token means pad left
        attention_mask = torch.tensor([[0, 1, 1], [1, 1, 1]])
        results = TextEncoder.mean_pool(last_hidden_states, attention_mask)
        expected = torch.cat(
            [
                (0.5 * last_hidden_states[0, -1, :] + 0.5 * last_hidden_states[0, -2, :]).unsqueeze(0),
                torch.mean(last_hidden_states[1, :, :], dim=0).unsqueeze(0),
            ],
            dim=0,
        )
        assert torch.allclose(results, expected)
