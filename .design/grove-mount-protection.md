# Grove Mount Protection: Agent Isolation from `.scion` Directory

**Status**: Proposal / Open for Review
**Date**: 2026-03-08

## Problem Statement

Agents running in containers can access the `.scion` directory of the grove they belong to. This directory contains other agents' home folders, which hold sensitive material:

- `.codex/auth.json` — raw API keys (written with `0600` perms)
- `.scion/secrets.json` — hub-projected variable secrets
- `.config/gcloud/application_default_credentials.json` — cloud credentials
- `.gemini/oauth_creds.json` — OAuth tokens
- Various harness-specific config files with auth fingerprints

Since all agents run with the same host UID (container user UID is synchronized to the host), `0600` file permissions provide **no isolation** between agents — any agent that can reach another agent's home directory can read its secrets.

### How Exposure Occurs

| Scenario | Mechanism | Severity |
|---|---|---|
| **Non-git workspace** | Project directory mounted at `/workspace`; `.scion/` is physically present | High — full access to all agent homes |
| **Full-repo fallback mount** (`common.go:169`) | When workspace is outside repo root, entire repo root mounted at `/repo-root` | High — `.scion/` accessible at `/repo-root/.scion/` |
| **Git worktree workspace** | Worktree excludes gitignored dirs; `.scion/` not materialized | Protected (incidentally, not by design) |

The git-worktree case is only protected because `.scion` is in `.gitignore` — this is an incidental side effect, not a security control.

### Current Mount Architecture

From `pkg/runtime/common.go`, `buildCommonRunArgs()`:

```
Agent home:  .scion/agents/<name>/home  →  /home/<user>     (bind mount, rw)
Workspace:   <worktree-path>            →  /repo-root/...   (bind mount, rw)
.git:        <repo>/.git                →  /repo-root/.git  (bind mount, rw)
```

When the workspace is outside the repo root (external worktrees, explicit `--workspace`):
```
Repo root:   <repo>/                    →  /repo-root       (bind mount, rw)  ← EXPOSES .scion/
Workspace:   <path>                     →  /workspace       (bind mount, rw)
```

---

## Proposed Approaches

### Approach 1: Externalize Grove Data (Non-Git Groves)

Move grove state out of the project directory entirely for non-git groves.

**Mechanism:**

When `scion init` runs in a non-git project:

1. Generate a UUID for the grove
2. Create grove data directory at `~/.scion/groves/<uuid>/.scion/` (agents, templates, settings)
3. Write a **marker file** at `<project>/.scion` (not a directory) containing:
   ```yaml
   grove-id: <uuid>
   grove-name: my-project
   ```
4. The container mounts only the project directory; the `.scion` marker file is inert

**Code impact:**

The following functions in `pkg/config/paths.go` check `info.IsDir()` and would need to handle the file-marker case:
- `FindProjectRoot()` (line 41)
- `ResolveGrovePath()` (line 198)
- `RequireGrovePath()` (line 231)
- `GetEnclosingGrovePath()` (line 261)

These would need to: detect `.scion` as a file, parse the grove-id, and resolve to `~/.scion/groves/<uuid>/.scion/`.

Additionally affected:
- `GetProjectAgentsDir()` / `GetProjectTemplatesDir()` — must follow the indirection
- `InitProject()` in `pkg/config/init.go` — rewrite to create file + external directory
- `GetGroveName()` — currently slugifies the parent directory; would need to read the marker file or continue deriving from the project directory name

**Pros:**
- Complete isolation: no sensitive data in the project directory
- Unifies with the hub-native grove model (already uses `~/.scion/groves/`)
- Container mount of the workspace is inherently safe

**Cons:**
- Breaking change requiring a migration path for existing groves
- When `rm -rf <project>`, the `~/.scion/groves/<uuid>/` data is orphaned (needs cleanup tooling)
- UUIDs in filesystem paths are not human-friendly (consider `<slug>-<short-uuid>` as compromise)

### Approach 2: Split Storage for Git Groves

For git-based groves, worktrees must remain inside the repo (they rely on `--relative-paths` for container mounting). The `.scion` directory cannot be fully externalized. Instead, split it:

**Mechanism:**

```
<repo>/.scion/                      (gitignored, remains a directory)
├── config.yaml                     settings, templates, grove config
├── settings.yaml
├── templates/
├── grove-id                        file with UUID for cross-referencing
└── agents/
    └── <name>/
        └── workspace/              git worktree (relative paths work)

~/.scion/groves/<uuid>/             (external, never mounted into containers)
└── agents/
    └── <name>/
        └── home/                   agent home with secrets
```

- **Worktree mechanics** stay in `<repo>/.scion/agents/<name>/workspace/` (because git relative paths require this)
- **Agent homes with secrets** move to `~/.scion/groves/<uuid>/agents/<name>/home/`
- The `config.HomeDir` in `RunConfig` already points to a specific path independent of workspace — changing it from `.scion/agents/<name>/home` to `~/.scion/groves/<uuid>/agents/<name>/home` is straightforward

**Code impact:**

- `pkg/agent/provision.go`: Change `agentHome` derivation to use external path
- `pkg/agent/run.go`: `HomeDir` in `RunConfig` already decoupled from workspace
- Agent deletion (`pkg/agent/delete.go`): Must clean up both locations
- `scion list`: Must reconcile state from two directories

**Pros:**
- Worktree compatibility preserved
- Agent homes (with secrets) completely outside the repo mount
- Git-based protection (gitignore) remains as defense-in-depth for config/templates

**Cons:**
- Split-brain: grove state in two locations complicates lifecycle management
- On multi-broker setups, `~/.scion/groves/<uuid>` would have different UUIDs on different machines for the same grove
- More complex cleanup on agent deletion

### Approach 3: Mount-Level Exclusion (Simpler Alternative for Git Groves)

Instead of splitting directories, fix the mount logic to never expose `.scion/` to containers.

**Mechanism:**

1. **Eliminate the full-repo fallback**: In `buildCommonRunArgs()` (`common.go:156-172`), always mount only `.git` + workspace, never the entire repo root. Adjust paths so git operations work with separated mounts.

2. **Shadow mount for safety**: When the full repo root must be mounted (if eliminating the fallback proves infeasible), add a tmpfs overlay:
   ```
   --mount type=tmpfs,destination=/repo-root/.scion
   ```
   This shadows the `.scion/` bind mount content with an empty tmpfs, making it invisible inside the container.

3. **Require `.scion` in `.gitignore`**: Enforce during `scion init` for git repos. Warn or error if `.scion` is not gitignored when starting agents.

**Code impact:**
- `pkg/runtime/common.go`: Modify the fallback branch (lines 165-172) or add tmpfs shadow mount
- `pkg/config/init.go`: Add `.gitignore` validation/enforcement
- Potentially `pkg/agent/run.go`: Add validation before agent start

**Pros:**
- Minimal code change — fixes the mount logic rather than restructuring storage
- No migration needed for existing groves
- No split-brain directory management

**Cons:**
- Defense depends on mount configuration correctness (not structural)
- `.gitignore` is a convention, not a hard guarantee (agents could modify it, history could contain `.scion/`)
- Non-git workspaces still need a different solution (Approach 1)

---

## Comparison Matrix

| Concern | Approach 1 (Externalize Non-Git) | Approach 2 (Split Git) | Approach 3 (Mount Exclusion) |
|---|---|---|---|
| Non-git grove protection | Strong | N/A | Partial (needs tmpfs shadow) |
| Git grove protection | N/A | Strong | Medium (mount-dependent) |
| Code complexity | Medium (~10 resolution sites) | Medium (provisioning + cleanup) | Low (~1-2 files) |
| Breaking change | Yes (migration needed) | Yes (home relocation) | No |
| Hub model convergence | Good (unifies with hub-native) | Partial | None |
| Defense model | Structural (data not present) | Structural (data not present) | Configurational (data masked) |
| Multi-broker compat | UUID divergence issue | UUID divergence issue | No issue |

---

## Recommended Combination

These approaches are not mutually exclusive. A layered strategy:

1. **Approach 3 (Mount Exclusion)** — Implement immediately as a low-effort fix. Eliminate the full-repo fallback mount or add tmpfs shadow. This closes the active vulnerability without breaking changes.

2. **Approach 1 (Externalize Non-Git)** — Implement for non-git groves as a structural improvement. This aligns with the hub-native grove model and provides strong isolation by design.

3. **Approach 2 (Split Git) vs Approach 3 (Mount Exclusion)** — For git groves, evaluate whether the structural split (Approach 2) is worth the complexity over the simpler mount-level fix (Approach 3). The mount fix may be sufficient given that git worktrees already exclude `.scion/` from the working tree.

---

## Open Questions

1. **UUID vs slug for grove paths**: Hub-native groves currently use slugs (`~/.scion/groves/<slug>/`). Should externalized linked groves also use slugs (risk of collision) or UUIDs (ugly paths)? A hybrid like `<slug>-<short-uuid>` could work.

2. **Marker file format**: Should `.scion` as a file be plain text (`grove-id: <uuid>`) or structured YAML? YAML is more extensible but heavier for a single field.

3. **Migration path**: Existing groves have `.scion/` as a directory with active agents. A `scion upgrade` or `scion migrate` command would be needed. Should this be automatic on first CLI invocation or explicit?

4. **Orphan cleanup**: When `.scion` is a marker file and the project directory is deleted, `~/.scion/groves/<uuid>/` is orphaned. Options: periodic GC, `scion prune`, or a grove registry that tracks liveness.

5. **Multi-broker UUID divergence**: If grove data lives at `~/.scion/groves/<uuid>/` on each broker, different brokers would have different UUIDs for the same grove. Should the UUID come from the Hub (consistent) or be generated locally? Hub-sourced UUIDs require Hub connectivity during init.

6. **Git history exposure**: Even with `.gitignore`, if `.scion/` was ever committed, it remains in git objects. Should `scion init` check for this and warn? Should agents be prevented from running `git show` on paths under `.scion/`?

7. **Hub API consolidation**: The scion CLI inside containers (`sciontool`) already routes most operations through the Hub API via `SCION_HUB_ENDPOINT`. Should we audit and ensure that *all* in-container CLI operations use the API, eliminating any need for filesystem access to grove data? This would make the mount-level protections defense-in-depth rather than the primary control.

8. **Local (non-hub) mode**: Approaches 1-3 all work without a Hub. But if we pursue API consolidation (question 7), local mode would need a lightweight local API server or accept weaker isolation. Is local mode a first-class security target?
