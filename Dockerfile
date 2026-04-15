# ─────────────────────────────────────────────────────────────────────────────
# miniagent — hardened sandbox image for the self-improvement evolution loop
#
# Security posture (enforced at runtime by `task evolve`):
#   --security-opt no-new-privileges   no setuid / capability escalation
#   --cap-drop ALL                     zero Linux capabilities
#   --read-only                        image layers are immutable
#   --tmpfs /tmp:rw,nosuid,size=512m   scratch space (exec allowed for go build)
#   -u 10000:10000 (miniagent)         never runs as root
#
# Go toolchain is bind-mounted read-only from the host at /usr/local/go.
# The full repo is bind-mounted read-write at /repo so the agent can:
#   - read its own source code
#   - edit agent/*.go and main.go
#   - run `go build` to compile a candidate binary
#   - run `git commit` on improvements
# ─────────────────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash \
      bc \
      ca-certificates \
      coreutils \
      git \
      jq \
 && rm -rf /var/lib/apt/lists/*

# Dedicated non-root user for the evolution loop
RUN groupadd -g 10000 miniagent \
 && useradd -u 10000 -g 10000 -m -d /home/miniagent -s /bin/bash miniagent

# Go is bind-mounted from host — just put it on PATH
ENV PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
ENV GOTOOLCHAIN=local

# Pre-built stable binary (the agent that will do the reasoning and judging).
# A copy lives inside the image so the first cycle has something to bootstrap from.
COPY .miniagent-bin /usr/local/bin/miniagent
RUN chmod 755 /usr/local/bin/miniagent

WORKDIR /repo
USER 10000

ENTRYPOINT ["/bin/bash"]
