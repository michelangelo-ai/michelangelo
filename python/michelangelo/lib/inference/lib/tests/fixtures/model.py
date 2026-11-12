import torch.nn as nn


class TorchModel(nn.Module):
    """Simple PyTorch model for testing."""

    def __init__(self, input_dim=10, output_dim=1):
        super().__init__()
        self.linear = nn.Linear(input_dim, output_dim)

    def forward(self, input):
        return self.linear(input)
