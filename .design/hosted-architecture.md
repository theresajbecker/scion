# Hosted Scion Architecture Design

## Status
**Proposed**

## 1. Overview
This document outlines the architecture for transforming Scion into a distributed platform supporting multiple runtime environments. The core goal is to separate the **State Management** (persistence/metadata) from the **Runtime Execution** (container orchestration).

The architecture introduces:
*   **Scion Hub (State Server):** A centralized API and database for agent state, templates, and users.
*   **Runtime Hosts:** Independent execution environments (local Docker, remote Kubernetes cluster, single server) that manage the actual agent containers.

This distributed model supports fully hosted SaaS scenarios, hybrid local/cloud setups, and "Solo Mode" (standalone CLI) using the same architectural primitives.

### Modes

#### Solo
This is using the scion CLI more or less in its traditional local way. The state storage is in the form of files in the agent folders, and labels on the running containers.

#### Read-only host

This is pretty much the same as solo, the source truth data is still stored the same as in Solo mode, but the CLI/Manager is reporting to a Hub endoint, and the sciontool also duel writes to both the local state system and to the hub.  Functions like 'scion list' are consulting the local state system. Sciontool starts an instance of itself in daemon mode for heartbeats

#### Connected

## 2. Goals & Scope
*   **Distributed Architecture:** Multiple Runtime Hosts can register with a single Scion Hub.
*   **Centralized State:** Agent metadata is persisted in a central database (Scion Hub), not just local files.
*   **Flexible Runtime:** Agents can run on local Docker, a remote server, or a Kubernetes cluster.
*   **Unified Interface:** Users interact with the Scion Hub API (or a CLI connected to it) to manage agents across any host.
*   **Web-Based Access:** Support for web-based PTY and management for hosted agents.

## 3. High-Level Architecture

```mermaid
graph TD
    User[User (CLI / Browser)] -->|HTTPS/WS| Hub[Scion Hub (State Server)]
    
    Hub -->|DB| DB[(Firestore/Postgres)]
    Hub -->|API| HostA[Runtime Host A (Kubernetes)]
    Hub -->|API| HostB[Runtime Host B (Docker/Local)]

    subgraph Runtime Host A
        PodA[Agent Pod]
    end

    subgraph Runtime Host B
        ContainerB[Agent Container]
    end

    HostA -->|Status/Events| Hub
    HostB -->|Status/Events| Hub

    User -.->|Direct PTY (Optional)| HostA
```

## 4. Core Components

### 4.1. Scion Hub (State Server)
The central authority responsible for:
*   **Persistence:** Stores `Agents`, `Groves` (Projects), `Users`, and `Templates`.
*   **Registration:** Tracks available Runtime Hosts.
*   **Routing:** Directs creation requests to the appropriate Runtime Host.
*   **API:** Exposes the primary REST/gRPC interface for clients.

### 4.2. Runtime Host
An execution node responsible for the CRUD lifecycle of agents.
*   **Interfaces:** Implements a standard API to `Start`, `Stop`, `Delete`, and `Attach` to agents.
*   **Runtime Providers:**
    *   **Kubernetes:** Orchestrates Pods/PVCs.
    *   **Docker/Container:** Orchestrates local containers.
*   **Operational Modes:**
    *   **Connected (Stateful):** Requires a persistent connection to the Scion Hub. The Hub has full control over the agent lifecycle on this host.
    *   **Read-only (Stateless):** An intermediate mode between Solo and Connected. The host is configured with Hub endpoint(s) and informs them of lifecycle events, but no direct control is available from the Scion Hub.
*   **Connectivity:** Can be co-located with the Hub or run remotely (requires secure tunnel/connection to Hub).
*   **Agent Communication:** Configures the `sciontool` inside agents to report status back to the Hub.

### 4.3. Grove (Project)
A logical grouping of agents.
*   **Distributed:** A Grove can span multiple Runtime Hosts.
*   **Identifier:** Uses a persistent ID in the Hub when distributed

### 4.4. Scion Tool (Agent-Side)
The agent-side helper script.
*   **Dual Reporting:** Reports status to the local Runtime Host *and* (if configured) the central Scion Hub.
*   **Identity:** Injected with `SCION_AGENT_ID` and `SCION_HUB_ENDPOINT`.

## 5. Detailed Workflows

### 5.1. Agent Creation (Hosted/Distributed)
1.  **User** requests agent creation via Scion Hub API.
2.  **Scion Hub**:
    *   Creates `Agent` record (Status: `PROVISIONING`).
    *   Selects a target **Runtime Host** (based on policy or user selection).
    *   Sends `CreateAgent` command to the Runtime Host.
3.  **Runtime Host**:
    *   Allocates resources (PVC, Container).
    *   Starts the Agent.
    *   Injects Hub connection details.
4.  **Agent**:
    *   Starts up.
    *   `sciontool` reports `RUNNING` status to Scion Hub.

### 5.2. Web PTY Attachment
1.  **User** connects to Scion Hub WebSocket.
2.  **Scion Hub** identifies the Runtime Host managing the agent.
3.  **Scion Hub** proxies the connection to the Runtime Host.
4.  **Runtime Host** streams the PTY from the container.

### 5.3. Standalone Mode (Solo)
*   The Scion CLI acts as both the **Hub** (using local file DB) and the **Runtime Host** (using Docker).
*   No external network dependencies required.

## 6. Migration & Compatibility
*   **Manager Interface:** The `pkg/agent.Manager` will be split/refined to support remote execution.
*   **Storage Interface:** Introduce `pkg/store` interface to abstract `sqlite` (local) vs `firestore` (hosted).