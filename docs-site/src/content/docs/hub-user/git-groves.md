---
title: "Tutorial: Git-Based Groves"
description: Set up a GitHub repository as a Hub-managed grove and dispatch agents to work on it remotely.
---

**What you will learn**: How to create a grove from a GitHub repository, configure authentication with a fine-grained Personal Access Token, and start agents that clone and work on your code — all without a local checkout.

## Local Worktrees vs. Hub Clones

Before diving in, it's important to understand the two workspace models Scion offers for git repositories:

| | Local Worktrees | Hub Clones (Git Groves) |
| :--- | :--- | :--- |
| **Where code lives** | On your machine, as a git worktree | Inside the agent container, cloned at startup |
| **Git credentials** | Uses your local git/SSH config | Requires a `GITHUB_TOKEN` secret on the Hub |
| **Isolation** | Worktree per agent, shared `.git` | Full clone per agent, completely independent |
| **Network** | No network needed (local files) | HTTPS clone from GitHub on each start |
| **Merging work** | Merge worktree branches locally | Agent pushes to remote; merge via Pull Request |
| **Hub required** | No | Yes |
| **Best for** | Solo development, fast iteration | Team workflows, remote brokers, CI-like dispatch |

**Key takeaway**: When a grove is managed by the Hub — whether created from a URL or linked from a local directory — agents always use **HTTPS clone-based provisioning**. Local worktrees are a local-mode feature only. This is intentional: the Hub enforces a consistent workspace strategy across all brokers and users.

For the full technical details on workspace strategies, see [About Workspaces](/scion/advanced-local/workspace/).

---

## Prerequisites

- A running Scion Hub that you are connected to (see [Connecting to Hub](/scion/hub-user/hosted-user/))
- A GitHub repository (public or private) you want agents to work on
- A GitHub account with permission to create fine-grained Personal Access Tokens

---

## Step 1: Create a GitHub Fine-Grained PAT

Scion uses a GitHub **fine-grained Personal Access Token (PAT)** to clone repositories over HTTPS inside agent containers. Fine-grained PATs are preferred over classic tokens because they can be scoped to specific repositories.

### Generate the Token

1. Go to **GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens**.
2. Click **Generate new token**.
3. Configure:
   - **Token name**: Something descriptive, e.g. `scion-agents-acme-backend`
   - **Expiration**: Choose an appropriate lifetime
   - **Repository access**: Select **Only select repositories** and pick the repo(s) your agents will work on
   - **Permissions**:
     - **Contents**: **Read and write** (required for clone and push)
     - **Pull requests**: **Read and write** (if you want agents to create PRs)
     - **Metadata**: **Read-only** (automatically selected)
4. Click **Generate token** and copy the value (it starts with `github_pat_`).

:::caution[Copy it now]
GitHub only shows the token value once. If you lose it, you'll need to regenerate it.
:::

---

## Step 2: Create a Git Grove

Create a grove on the Hub from your repository URL:

```bash
scion hub grove create https://github.com/acme/backend.git
```

Scion will auto-detect the default branch and derive a slug from the repository name:

```
Grove created:
  ID:     a1b2c3d4e5f67890
  Slug:   acme-backend
  Remote: github.com/acme/backend
  Branch: main
```

### Optional Flags

```bash
# Specify a branch (useful for long-lived feature branches)
scion hub grove create https://github.com/acme/backend.git --branch develop

# Override the auto-generated slug
scion hub grove create https://github.com/acme/backend.git --slug my-backend
```

:::tip[Idempotent creation]
Creating a grove from the same git URL twice won't create a duplicate — the grove ID is derived from the normalized URL, so the command is idempotent.
:::

---

## Step 3: Upload Your Token as a Secret

Store the GitHub PAT as a grove-scoped secret so that all agents in this grove can authenticate:

```bash
scion hub secret set --grove acme-backend GITHUB_TOKEN github_pat_xxxxxxxx
```

Secrets are **write-only** — the value is encrypted and can never be read back via the CLI or API. It is only decrypted at runtime and injected into the agent container as an environment variable.

### User-Scoped vs. Grove-Scoped

You can also set `GITHUB_TOKEN` at the **user scope** if your token covers multiple repositories:

```bash
# User-scoped: available to all your agents across all groves
scion hub secret set GITHUB_TOKEN github_pat_xxxxxxxx
```

Grove-scoped secrets take priority over user-scoped ones (see [Secret Management](/scion/hub-user/secrets/) for the full resolution hierarchy).

### Web Dashboard Alternative

You can also upload secrets through the Web Dashboard:
1. Navigate to the grove's settings page.
2. Open the **Secrets** tab.
3. Click **Add Secret**, enter `GITHUB_TOKEN` as the key, paste the token value, and save.

---

## Step 4: Start an Agent

With the grove and token in place, start an agent targeting the grove:

```bash
scion start my-agent --grove acme-backend "add input validation to the /users endpoint"
```

You'll see the agent go through the clone-based startup:

```
Using hub, cloning repo https://github.com/acme/backend.git
  (Hub mode uses HTTPS clone with GITHUB_TOKEN; local worktrees are not used)

Agent 'my-agent' starting on broker 'us-west-01'...
  Status: CLONING (github.com/acme/backend @ main)
  Status: STARTING
  Status: RUNNING
```

### What happens inside the container

1. The repository is shallow-cloned (`depth=1`) into `/workspace`
2. A feature branch `scion/my-agent` is created and checked out
3. Git identity is configured automatically
4. The agent's harness (Claude, Gemini, etc.) starts with the task prompt

### Running multiple agents

Each agent gets its own clone and its own `scion/<agent-name>` branch, so you can run several in parallel on the same grove:

```bash
scion start agent-auth    --grove acme-backend "add OAuth2 support"
scion start agent-tests   --grove acme-backend "increase test coverage for pkg/api"
scion start agent-docs    --grove acme-backend "update the API documentation"
```

---

## Step 5: Monitor and Retrieve Work

### Check agent status

```bash
scion list --grove acme-backend
```

### Attach to an agent

If an agent is waiting for input or you want to observe it:

```bash
scion attach my-agent --grove acme-backend
```

### Retrieve the code

Since agents push their branches to the remote, you can pull their work from any machine:

```bash
git fetch origin scion/my-agent
git log origin/scion/my-agent
```

Or create a Pull Request directly from the branch on GitHub.

---

## Linked Groves: When Local Becomes Remote

If you already have a local checkout linked to the Hub via `scion hub link`, be aware that **the workspace strategy changes**. Once linked, even if the broker machine has the repository on disk, agents use clone-based provisioning — not local worktrees.

This means you need a `GITHUB_TOKEN` secret set on the grove, just like a grove created from a URL.

To temporarily fall back to local worktree mode:

```bash
# Disable hub for a single command
scion start my-agent --no-hub "quick local fix"

# Or disable hub integration entirely
scion hub disable
```

---

## Troubleshooting

### Agent fails with "authentication required" or "clone failed"

- Verify a `GITHUB_TOKEN` is set on the grove or at user scope:
  ```bash
  scion hub secret get --grove acme-backend
  ```
- Ensure the token has **Contents: Read** permission at minimum.
- Check that the token has not expired.
- For fine-grained PATs, confirm the target repository is included in the token's repository access list.

### Agent clones the wrong branch

Use the `--branch` flag when creating the grove to set the default:

```bash
scion hub grove create https://github.com/acme/backend.git --branch develop
```

### Agent needs full git history

Shallow clones (`depth=1`) are used by default for fast startup. If an agent needs full history for operations like `git log` or `git blame`, it can fetch the rest from inside its session:

```bash
git fetch --unshallow
```
