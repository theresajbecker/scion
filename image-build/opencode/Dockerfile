# syntax=docker/dockerfile:1
ARG BASE_IMAGE
FROM ${BASE_IMAGE}

# Install OpenCode
# RUN curl -fsSL https://opencode.ai/install | bash
RUN npm install -g opencode-ai \
  && npm cache clean --force
# Ensure opencode is in PATH
# ENV PATH="/home/scion/.opencode/bin:${PATH}"

CMD ["opencode"]
