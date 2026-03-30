FROM mcr.microsoft.com/powershell:7.4-ubuntu-22.04

# Avoid prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    ca-certificates \
    openssh-client \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 22 LTS
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.26
RUN curl -fsSL https://go.dev/dl/go1.26.1.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

# Install Claude CLI and pnpm
RUN npm install -g @anthropic-ai/claude-code pnpm

# Install golangci-lint (needed for agent CI checks)
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.10.1

# Create workspace
RUN mkdir -p /workspace /agent

# Copy PC agent scripts
COPY scripts/pc-agent/ /agent/scripts/pc-agent/

# Copy start script
COPY deploy/pc-agent/start.sh /agent/start.sh
RUN chmod +x /agent/start.sh

WORKDIR /workspace

# The container runs the agent loop continuously
CMD ["/agent/start.sh"]
