#!/bin/bash

# Fix Cadence CLI for Python subprocess compatibility
# This script creates a wrapper that allows subprocess to use cadence via Docker

echo "Creating cadence wrapper for subprocess compatibility..."

# Remove any existing broken cadence binary
if [ -f /usr/local/bin/cadence ]; then
    echo "Removing existing cadence binary..."
    sudo rm -f /usr/local/bin/cadence
fi

# Create the wrapper script
echo "Creating cadence wrapper script..."
sudo tee /usr/local/bin/cadence > /dev/null << 'EOF'
#!/bin/bash
docker run --rm -i ubercadence/cli:v1.2.6 cadence "$@"
EOF

# Make it executable
echo "Making wrapper script executable..."
sudo chmod +x /usr/local/bin/cadence

# Test the installation
echo "Testing cadence wrapper..."
if cadence --help > /dev/null 2>&1; then
    echo "✓ Cadence wrapper created successfully!"
    echo "✓ Both terminal and Python subprocess can now use 'cadence' command"
else
    echo "✗ Failed to create cadence wrapper"
    exit 1
fi

echo "Done! Python subprocess should now work with cadence CLI."