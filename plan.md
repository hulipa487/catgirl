# Catgirl Agentic Runtime - Revised Architecture Plan (v4.0)

## Document Purpose

This document describes the **revised architecture** for the Catgirl Agentic Runtime system. It is **language-agnostic** and focuses on the task-based agent architecture with autonomous capability enhancement.

**Key Changes from v3.x:**
- Main Orchestrator: One per session (unchanged)
- Task Queue: **GLOBAL** (shared across all sessions)
- Agent Pool: **GLOBAL** (shared across all sessions)
- Container Snapshots: Generated on task exit for later recall

---

## 1. System Overview

### 1.1 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         GLOBAL RESOURCES (Shared)                            │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         GLOBAL TASK QUEUE                               │ │
│  │  Contains tasks from ALL sessions, prioritized across sessions         │ │
│  │                                                                        │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐                  │ │
│  │  │Task(S1)  │ │Task(S2)  │ │Task(S1)  │ │Task(S3)  │  ...            │ │
│  │  │priority:HIGH│ │priority:NORM│ │priority:CRIT│ │priority:LOW│        │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘                  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                       GLOBAL AGENT POOL                                 │ │
│  │  Workers available to ANY session's tasks                              │ │
│  │                                                                        │ │
│  │   ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐               │ │
│  │   │Agent1│ │Agent2│ │Agent3│ │Agent4│ │Agent5│ │ ...  │               │ │
│  │   │ GP   │ │ GP   │ │ Reas │ │ GP   │ │ Reas │ │      │               │ │
│  │   └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘               │ │
│  │                                                                        │ │
│  │   Agents pick highest priority task from GLOBAL queue                 │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                   CONTAINER SNAPSHOT STORE                              │ │
│  │  Stores container snapshots for all completed/exited tasks             │ │
│  │                                                                        │ │
│  │  snapshot_id → task_id → container state (image + volumes)            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                              ▲
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│   Session #1     │ │   Session #2     │ │   Session #3     │
│                  │ │                  │ │                  │
│ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
│ │ Orchestrator │ │ │ │ Orchestrator │ │ │ │ Orchestrator │ │
│ │ (per session)│ │ │ │ (per session)│ │ │ │ (per session)│ │
│ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
│                  │ │                  │ │                  │
│ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
│ │ Working Mem  │ │ │ │ Working Mem  │ │ │ │ Working Mem  │ │
│ │ (per agent)  │ │ │ │ (per agent)  │ │ │ │ (per agent)  │ │
│ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
│                  │ │                  │ │                  │
│ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
│ │ Long-Term    │ │ │ │ Long-Term    │ │ │ │ Long-Term    │ │
│ │ Memory       │ │ │ │ Memory       │ │ │ │ Memory       │ │
│ │ (per session)│ │ │ │ (per session)│ │ │ │ (per session)│ │
│ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
│                  │ │                  │ │                  │
│ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
│ │ MCP Client   │ │ │ │ MCP Client   │ │ │ │ MCP Client   │ │
│ │ (per session)│ │ │ │ (per session)│ │ │ │ (per session)│ │
│ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
│                  │ │                  │ │                  │
│ ┌──────────────┐ │ │ ┌──────────────┐ │ │ ┌──────────────┐ │
│ │ Skills       │ │ │ │ Skills       │ │ │ │ Skills       │ │
│ │ (per session)│ │ │ │ (per session)│ │ │ │ (per session)│ │
│ └──────────────┘ │ │ └──────────────┘ │ │ └──────────────┘ │
└──────────────────┘ └──────────────────┘ └──────────────────┘
```

### 1.2 Key Concepts

| Concept | Scope | Description |
|---------|-------|-------------|
| **Session** | Per-user | Isolated environment with own orchestrator and memory |
| **Main Orchestrator** | Per-session | Coordinator for that session's tasks |
| **Working Memory** | Per-agent | Each agent's private short-term context (READ/WRITE by owner agent only) |
| **Long-Term Memory** | Per-session | READ-ONLY for all agents; runtime auto-consolidates working memory |
| **Short-Term Memory** | Per-agent | Same as Working Memory (alias) |
| **Context** | Per-agent | Same as Working Memory (alias) |
| **MCP Client** | Per-session | Each session has its own MCP client instance and servers |
| **Skills** | Per-session | Each session has its own skill registry |
| **Task Queue** | **GLOBAL** | Single queue for ALL tasks across ALL sessions |
| **Agent Pool** | **GLOBAL** | Shared workers available to ANY session |
| **task_id** | Per-task-family | Identifies a task family (shared by parent and sub-tasks) |
| **instance_id** | Per-instance | Unique identifier for each task instance |
| **owner_id** | Per-instance | Agent ID to receive results/communications |
| **Container Snapshot** | Global | Saved state of container on task exit |

---

## 2. Component Specifications

### 2.1 Main Orchestrator (Per-Session, Limited)

**Design Principle:** Each session has exactly ONE Main Orchestrator that coordinates work for that user. It cannot perform work directly - it spawns tasks to the GLOBAL queue.

**Capabilities:**

| Resource | Access | Purpose |
|----------|--------|---------|
| Working Memory (own agent) | READ/WRITE | Store context, track progress (private to this agent, isolated) |
| Long-Term Memory (own session) | READ-ONLY | Recall past information (auto-populated by runtime consolidation) |
| MCP Client (own session) | CALL_TOOL, ADD_SERVER | Use tools from session's MCP servers |
| SPAWN_TASK action | YES | Create tasks in GLOBAL queue |
| All other actions | NO | Cannot directly execute, call skills, etc. |

**Responsibilities:**
1. Receive user messages from Telegram
2. Analyze requests and determine required work
3. Spawn tasks to **GLOBAL** task queue (with session_id for isolation)
4. Monitor task completion via working memory
5. Aggregate results from completed tasks
6. Send responses to user

### 2.1.1 Per-Session MCP Client

**Design Principle:** Each session has its OWN MCP client instance. MCP servers connected in one session are NOT accessible from other sessions.

**MCP Client Properties:**
```
SessionMCPClient {
    session_id: UUID
    servers: map[server_name → MCPServer]
    tools: map[tool_name → Tool]
    status: enum (CONNECTED, DISCONNECTED, ERROR)
}
```

**MCP Server (Per-Session):**
```
MCPServer {
    server_id: UUID
    session_id: UUID (MUST match session)
    name: string
    connection_type: enum (STDIO, HTTP, WEBSOCKET)
    connection_string: string
    status: enum (CONNECTED, DISCONNECTED, ERROR)
    tools: list[Tool]
}
```

**Operations:**
```
# Add MCP server to session
add_mcp_server(session_id, server_config) -> server_id:
    server = MCPServer(
        server_id = generate_uuid(),
        session_id = session_id,  # Isolated to this session
        name = server_config.name,
        connection_type = server_config.type,
        connection_string = server_config.connection_string
    )
    connect_server(server)
    discover_tools(server)
    save_to_database(server)
    return server.server_id

# Call tool (only from session's own MCP servers)
call_tool(session_id, tool_name, arguments) -> result:
    # Verify tool belongs to session's MCP servers
    server = get_server_for_tool(session_id, tool_name)
    if server.session_id != session_id:
        raise PermissionError("Cannot access MCP server from another session")

    return invoke_tool(server, tool_name, arguments)

# List tools available to session
list_session_tools(session_id) -> list[Tool]:
    servers = get_session_mcp_servers(session_id)
    tools = []
    for server in servers:
        tools.extend(server.tools)
    return tools
```

**Session Isolation:**
```
Session A                          Session B
│                                  │
├─ MCP Client A                    ├─ MCP Client B
│  ├─ Server: Web Search           │  ├─ Server: File System
│  ├─ Server: Database             │  ├─ Server: GitHub
│  └─ Tools: [search, query]       │  └─ Tools: [read, write, commit]
│                                  │
│ Agent from Session A             │ Agent from Session B
│ CANNOT access Session B's MCP    │ CANNOT access Session A's MCP
```

### 2.2 Global Task Queue

**Design Principle:** Single priority queue shared across ALL sessions. Tasks are prioritized globally, not per-session.

**Properties:**
```
GlobalTaskQueue {
    queue: list[TaskInstance] (priority-sorted across ALL sessions)
    max_queue_size: integer
    max_depth: integer (configured globally)
}
```

**Priority Calculation:**
```
calculate_priority(task) -> integer:
    base_priority = task.priority  # LOW=0, NORMAL=1, HIGH=2, CRITICAL=3

    # Session boost (optional: premium users get boost)
    session_boost = get_session_priority_boost(task.session_id)

    # Age boost (older tasks get slight boost)
    age_boost = min(task.age_minutes / 60, 1)  # Max 1 point boost

    return base_priority + session_boost + age_boost
```

**Operations:**
```
enqueue(task) -> void:
    if task.depth >= max_depth:
        reject("Task depth exceeds maximum")

    priority = calculate_priority(task)
    task.priority_score = priority
    queue.add(task)
    sort_by(priority_score DESC, created_at ASC)

dequeue(agent_type) -> TaskInstance | null:
    # Find highest priority task matching agent type
    for task in queue:
        if task.agent_type == agent_type:
            queue.remove(task)
            return task

    # No matching type, take highest priority any task
    if queue not empty:
        return queue.remove_first()

    return null

get_queue_status() -> object:
    return {
        total_tasks: queue.length,
        by_session: count_by_session(queue),
        by_priority: count_by_priority(queue),
        by_depth: count_by_depth(queue),
        by_type: count_by_agent_type(queue)
    }
```

### 2.3 Global Agent Pool

**Design Principle:** All worker agents are in a single global pool, available to pick up tasks from ANY session.

**Properties:**
```
GlobalAgentPool {
    agents: list[WorkerAgent]
    min_agents: integer
    max_agents: integer
    gp_agent_ratio: float
    idle_timeout_seconds: integer
    max_tasks_per_agent: integer
}
```

**Agent Assignment Logic:**
```
assign_task_to_agent(task) -> WorkerAgent | null:
    # Find idle agent matching task type
    for agent in pool.agents:
        if agent.status == IDLE and agent.type == task.agent_type:
            agent.status = BUSY
            agent.current_instance = task
            return agent

    # No matching type, use any idle agent
    for agent in pool.agents:
        if agent.status == IDLE:
            agent.status = BUSY
            agent.current_instance = task
            return agent

    # No idle agents, task stays in queue
    return null

# Agents poll the global queue
agent_poll_queue(agent):
    while agent.status == IDLE:
        task = global_queue.dequeue(agent.type)
        if task:
            agent.status = BUSY
            agent.current_instance = task
            agent.execute(task)
        else:
            sleep(1)  # No tasks, wait
```

### 2.4 Task Instance

**Properties:**
```
TaskInstance {
    instance_id: UUID (unique identifier for THIS instance)
    task_id: UUID (shared with parent and all sub-tasks)
    session_id: UUID (owning session - for isolation)
    owner_id: string (agent_id who owns this task - receives results)
    depth: integer (0 = root, increases for sub-tasks)
    description: string (detailed task description)
    agent_type: enum (GENERAL_PURPOSE, REASONER)
    status: enum (PENDING, ASSIGNED, IN_PROGRESS, COMPLETED, FAILED, EXITED)
    priority: enum (LOW, NORMAL, HIGH, CRITICAL)
    priority_score: float (calculated for global queue ordering)
    created_at: timestamp
    assigned_agent_id: string | null
    started_at: timestamp | null
    completed_at: timestamp | null
    result: any | null
    error: string | null
    parent_instance_id: UUID | null (for tree tracking)
    container_snapshot_id: string | null (snapshot on exit)
}
```

### 2.5 Container Snapshot System

**Design Principle:** When a task exits (completes, fails, or is interrupted), the runtime creates a snapshot of the container state for later recall and inspection.

**Snapshot Contents:**
```
ContainerSnapshot {
    snapshot_id: UUID (unique identifier)
    task_id: UUID (task family this snapshot belongs to)
    instance_id: UUID (specific instance that triggered snapshot)
    session_id: UUID (owning session)
    container_id: string (original container ID)
    created_at: timestamp
    reason: enum (COMPLETED, FAILED, EXITED, INTERRUPTED)

    # Snapshot data
    image: {
        name: string (e.g., "snapshot_task_A6_20260321")
        size_bytes: integer
        layers: list[string]
    }
    volumes: {
        "/workspace": {
            files: list[FileInfo],
            total_size_bytes: integer
        }
    }
    environment: map[string]string (env vars at snapshot time)
    metadata: {
        agent_id: string (agent that was executing)
        execution_time_seconds: float
        memory_used_bytes: integer
        cpu_time_seconds: float
    }
}
```

**Snapshot Lifecycle:**
```
1. Task exits (any reason)
   │
   ▼
2. Runtime pauses task cleanup
   │
   ▼
3. Create container snapshot
   - Commit container to image
   - Save volume state
   - Record metadata
   │
   ▼
4. Store snapshot in snapshot store
   - snapshot_id → task_id mapping
   - Index by session_id for retrieval
   │
   ▼
5. Release running container
   - Stop and remove running container
   - Snapshot persists
   │
   ▼
6. Snapshot available for recall
   - Debug/inspection
   - Resume execution
   - Audit trail
```

**Snapshot API:**
```
create_snapshot(instance, reason) -> snapshot_id:
    container_id = get_container_for_task(instance.task_id)

    # Commit container to image
    image_name = "snapshot_{task_id}_{timestamp}"
    image_id = docker.commit(container_id, image_name)

    # Export volumes
    volumes = export_volumes(container_id)

    # Collect metadata
    stats = get_container_stats(container_id)

    # Create snapshot record
    snapshot = ContainerSnapshot(
        snapshot_id = generate_uuid(),
        task_id = instance.task_id,
        instance_id = instance.instance_id,
        session_id = instance.session_id,
        container_id = container_id,
        reason = reason,
        image = image_id,
        volumes = volumes,
        metadata = stats
    )

    snapshot_store.save(snapshot)
    instance.container_snapshot_id = snapshot.snapshot_id

    return snapshot.snapshot_id

recall_snapshot(snapshot_id) -> container_id:
    snapshot = snapshot_store.get(snapshot_id)

    # Create new container from snapshot image
    container = docker.create_container(
        image = snapshot.image,
        volumes = snapshot.volumes,
        environment = snapshot.environment
    )

    return container.id

list_snapshots(filters) -> list[ContainerSnapshot]:
    return snapshot_store.query(filters)
    # Filters: session_id, task_id, date_range, reason
```

**Snapshot Retention Policy:**
```
Snapshot Retention:
- COMPLETED tasks: Keep for 7 days
- FAILED tasks: Keep for 30 days (for debugging)
- INTERRUPTED tasks: Keep for 14 days
- CRITICAL tasks: Keep indefinitely (configurable)

Cleanup Job (runs daily):
    expired = snapshot_store.find_expired(retention_policy)
    for snapshot in expired:
        docker.remove_image(snapshot.image)
        snapshot_store.delete(snapshot)
```

---

### 2.6 Per-Agent Working Memory

**Design Principle:** Each agent (Main Orchestrator + Worker Agents) has its OWN private working memory. Working memory is NOT shared across agents, even within the same session.

**Working Memory Properties:**
```
AgentWorkingMemory {
    agent_id: string (unique identifier of the agent)
    session_id: UUID (for cross-reference, but data is NOT shared)
    entries: map[key → value]
    max_entries: integer (configurable limit)
}
```

**Operations:**
```
# Set value in agent's own working memory
set(agent_id, key, value) -> void:
    db.execute(
        "INSERT INTO working_memory (agent_id, key, value, updated_at)
         VALUES ($1, $2, $3, NOW())
         ON CONFLICT (agent_id, key)
         DO UPDATE SET value = $3, updated_at = NOW()",
        agent_id, key, value
    )

# Get value from agent's own working memory
get(agent_id, key) -> value | null:
    result = db.fetchone(
        "SELECT value FROM working_memory WHERE agent_id = $1 AND key = $2",
        agent_id, key
    )
    return result.value if result else null

# Delete from agent's own working memory
delete(agent_id, key) -> boolean:
    result = db.execute(
        "DELETE FROM working_memory WHERE agent_id = $1 AND key = $2",
        agent_id, key
    )
    return result.rows_affected > 0

# List all keys in agent's working memory
list_keys(agent_id) -> list[string]:
    results = db.fetch(
        "SELECT key FROM working_memory WHERE agent_id = $1 ORDER BY key",
        agent_id
    )
    return [r.key for r in results]

# Copy data from one agent's working memory to another (explicit sharing)
copy_to_agent(from_agent_id, to_agent_id, keys) -> void:
    for key in keys:
        value = get(from_agent_id, key)
        if value is not null:
            set(to_agent_id, key, value)
```

**Memory Isolation:**
```
Agent A (session S1)              Agent B (session S1)
│                                  │
├─ Working Memory A                ├─ Working Memory B
│  ├─ key1: "value1"              │  ├─ key1: "different_value"
│  └─ key2: "value2"              │  └─ task_context: "specific info"
│                                  │
│ CANNOT access Working Memory B   │ CANNOT access Working Memory A
│                                  │
│ To share data:                   │
│ 1. Use Long-Term Memory (shared per session)
│ 2. Explicitly copy via copy_to_agent()
│ 3. Write to task result (visible to owner)
```

**Agent Lifecycle and Working Memory:**
```
1. Agent created
   │
   ▼
2. Working memory initialized (empty)
   - agent_id assigned
   │
   ▼
3. Agent executes tasks
   - Stores task context in own working memory
   - Stores intermediate results in own working memory
   │
   ▼
4. Agent completes task
   - Results written to task result (visible to owner)
   - Working memory persists (agent may be reused)
   │
   ▼
5. Agent idle or reassigned
   - Working memory cleared on reassignment (optional)
   - Or retained for agent state preservation
```

**Sharing Data Between Agents (Same Session):**
```
# Option 1: Long-Term Memory (shared per session)
write_long_term(session_id, content, metadata)
results = search_long_term(session_id, query)

# Option 2: Explicit copy (agent-to-agent)
copy_to_agent(
    from_agent_id = "Agent_A",
    to_agent_id = "Agent_B",
    keys = ["context", "important_data"]
)

# Option 3: Task result (visible to owner, then owner can spawn new task)
task.result = {"summary": "...", "details": "..."}
notify_owner(owner_id, task.result)
```

---

### 2.7 Automated Long-Term Memory Consolidation

**Design Principle:** Long-term memory is READ-ONLY for ALL agents. The runtime automatically consolidates working memories from all agents in a session into long-term memory using a multi-tier summarization strategy.

**Terminology:**
- **Working Memory** = **Short-Term Memory** = **Context** (all refer to per-agent private memory)
- **Long-Term Memory** = Shared, read-only memory auto-populated by runtime

**Consolidation Pipeline:**
```
┌─────────────────────────────────────────────────────────────────────────┐
│              Long-Term Memory Consolidation Pipeline                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Working Memories (Per-Agent)                                           │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐                   │
│  │ Agent 1  │ │ Agent 2  │ │ Agent 3  │ │   ...    │                   │
│  │ WM entries│ │ WM entries│ │ WM entries│ │          │                   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘                   │
│         │            │            │                                     │
│         └────────────┴────────────┘                                     │
│                   │                                                      │
│                   ▼                                                      │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  Memory Consolidation Service (Background Process)                 │ │
│  │                                                                    │ │
│  │  1. Scan all working memories (session-scoped)                     │ │
│  │  2. Identify frequently accessed entries                           │ │
│  │  3. Generate embeddings for semantic search                        │ │
│  │  4. Store in long-term memory                                      │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                   │                                                      │
│                   ▼                                                      │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │  Long-Term Memory (Per-Session, READ-ONLY for agents)              │ │
│  │                                                                    │ │
│  │  Tier 1: Raw Entries (recent, high-frequency)                      │ │
│  │     - Full working memory entries                                  │ │
│  │     - Embedding: full vector                                       │ │
│  │     - Retention: 7 days                                            │ │
│  │                                                                    │ │
│  │  Tier 2: Summarized Entries (medium-frequency)                     │ │
│  │     - LLM-generated summaries of related entries                   │ │
│  │     - Embedding: summary vector                                    │ │
│  │     - Retention: 30 days                                           │ │
│  │                                                                    │ │
│  │  Tier 3: Brief Descriptions (low-frequency, archival)              │ │
│  │     - Highly condensed descriptions                                │ │
│  │     - Embedding: brief vector                                      │ │
│  │     - Retention: 90 days                                           │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
```

**Consolidation Process:**
```
Every N minutes (configurable, default 5):

1. SCAN working memories for all agents in session
   entries = scan_working_memory(session_id)

2. FILTER frequently accessed entries
   frequent = filter_by_access_count(entries, min_access=3)

3. GENERATE embeddings
   for entry in frequent:
       entry.embedding = generate_embedding(entry.value)

4. STORE in long-term memory (Tier 1)
   store_long_term_memory(session_id, frequent, tier=1)

5. CONSOLIDATE old Tier 1 entries (not accessed in 7 days)
   old_entries = get_tier_entries(session_id, tier=1, age_days=7)
   summary = llm_summarize(old_entries)  # LLM generates summary
   summary.embedding = generate_embedding(summary)
   store_long_term_memory(session_id, summary, tier=2)
   delete_long_term_memory(session_id, old_entries)

6. CONSOLIDATE old Tier 2 entries (not accessed in 30 days)
   old_summary = get_tier_entries(session_id, tier=2, age_days=30)
   brief = llm_brief_summarize(old_summary)  # Highly condensed
   brief.embedding = generate_embedding(brief)
   store_long_term_memory(session_id, brief, tier=3)
   delete_long_term_memory(session_id, old_summary)

7. DELETE old Tier 3 entries (older than 90 days)
   expired = get_tier_entries(session_id, tier=3, age_days=90)
   delete_long_term_memory(session_id, expired)
```

**Long-Term Memory Schema:**
```sql
CREATE TABLE long_term_memories (
    id UUID PRIMARY KEY,
    session_id UUID REFERENCES sessions(id),
    tier VARCHAR(20) NOT NULL,  -- 'tier1_raw', 'tier2_summary', 'tier3_brief'
    content TEXT NOT NULL,
    embedding VECTOR(1024),  -- For semantic search
    access_count INTEGER DEFAULT 0,
    last_accessed_at TIMESTAMPTZ,
    source_agent_ids JSONB,  -- Which agents' working memory contributed
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);
CREATE INDEX idx_memories_session ON long_term_memories(session_id);
CREATE INDEX idx_memories_tier ON long_term_memories(tier);
CREATE INDEX idx_memories_embedding ON long_term_memories USING ivfflat(embedding cosine_ops);
CREATE INDEX idx_memories_access ON long_term_memories(access_count DESC, last_accessed_at);
CREATE INDEX idx_memories_expires ON long_term_memories(expires_at);
```

**Agent Access (READ-ONLY):**
```
# Search long-term memory (all agents can do this)
search_long_term_memory(session_id, query, top_k=5) -> list[MemoryEntry]:
    # Generate embedding for query
    query_embedding = generate_embedding(query)

    # Semantic search across all tiers
    results = db.fetch(
        """
        SELECT id, content, tier, access_count,
               1 - (embedding <=> $1) AS similarity
        FROM long_term_memories
        WHERE session_id = $2
        ORDER BY similarity DESC
        LIMIT $3
        """,
        query_embedding, session_id, top_k
    )

    # Increment access count (for retention tracking)
    for result in results:
        db.execute(
            "UPDATE long_term_memories SET access_count = access_count + 1,
             last_accessed_at = NOW() WHERE id = $1",
            result.id
        )

    return results

# Get specific memory by ID (read-only)
get_long_term_memory(session_id, memory_id) -> MemoryEntry | null:
    result = db.fetchone(
        "SELECT * FROM long_term_memories WHERE id = $1 AND session_id = $2",
        memory_id, session_id
    )

    if result:
        # Increment access count
        db.execute(
            "UPDATE long_term_memories SET access_count = access_count + 1,
             last_accessed_at = NOW() WHERE id = $1",
            result.id
        )
        return result

    return null
```

---

### 2.8 Simple RAG Retrieval

**Design Principle:** Retrieval happens automatically before every LLM call. No triggers, no explicit actions—the system proactively fetches relevant memories based on current context.

**Pre-LLM Automatic Retrieval Flow:**
```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Pre-LLM Automatic Retrieval                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Thought-Action Loop starts iteration                                 │
│     │                                                                    │
│     ▼                                                                    │
│  2. Build context from working memory                                    │
│     │                                                                    │
│     ▼                                                                    │
│  3. Extract key terms from context                                       │
│     │                                                                    │
│     ▼                                                                    │
│  4. Auto-search Long-Term Memory                                         │
│     │                                                                    │
│     ▼                                                                    │
│  5. Inject retrieved memories into LLM prompt                            │
│     │                                                                    │
│     ▼                                                                    │
│  6. Call LLM with augmented context                                      │
│     │                                                                    │
│     ▼                                                                    │
│  7. LLM generates thought + action with full context                     │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Pre-LLM Retrieval Process:**

```
Step 1: Build Base Context
│
├─► Get working memory entries
├─► Get conversation history
└─► Combine into context structure

Step 2: Auto-Retrieve (if enabled)
│
├─► Extract key terms from context
├─► Search LTM for each term
├─► Deduplicate results
└─► Add to context as "retrieved_memories"

Step 3: Call LLM
│
├─► Send augmented context
└─► Receive thought + action response

Step 4: Parse and Execute
│
├─► Parse thought from response
├─► Parse action from response
├─► Execute action
└─► Return results
```

**Auto-Retrieve Process:**

```
Extract Key Terms
│
├─► Analyze working memory keys
├─► Select top N terms (default 3)
│
▼
Search LTM Per Term
│
├─► For each term:
│   ├─► Generate query embedding
│   └─► Search LTM (top_k results)
│
▼
Combine Results
│
├─► Merge all results
├─► Remove duplicates
├─► Limit to max results
│
▼
Return to Context
```
    def _extract_key_terms(self, text: str, top_n: int) -> list[str]:
        """Extract key terms from text for retrieval."""
        # Extract: recent working memory keys, entities, keywords
        return extract_keywords(text, top_n)
```

**Configuration:**
```json
{
    "rag": {
        "enabled": true,
        "default_top_k": 5,
        "embedding_model": "BAAI/bge-m3",

        "auto_retrieve": {
            "enabled": true,
            "on_llm_call": true,
            "top_k": 3,
            "max_results": 10,
            "min_similarity": 0.7
        }
    }
}
```

**Example:**
```
Working Memory contains:
  - "current_task": "Build authentication system"
  - "database": "PostgreSQL"
  - "auth_method": "JWT"

Auto-retrieve extracts key terms: ["authentication", "PostgreSQL", "JWT"]

Searches LTM for each term, retrieves:
  [1] "User wants PostgreSQL database" (similarity: 0.92)
  [2] "Auth system should use JWT tokens" (similarity: 0.89)
  [3] "API endpoints: /users, /auth, /data" (similarity: 0.75)

LLM prompt includes all of the above automatically.
```

---

### 2.8.1 Conversation History Management

**Design Principle:** Keep ALL conversation history in context. When context window reaches a threshold, automatically spawn a compaction task to summarize old history while preserving recent turns.

**Context Window Management:**
```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Context Window Management                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Max Context Window: 128,000 tokens (configurable)                       │
│  Compaction Threshold: 80% (configurable)                                │
│  Preserve Recent: 20 turns (configurable)                                │
│                                                                          │
│  Context Structure:                                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ 1. System Prompt (~500 tokens)                                      ││
│  │ 2. Working Memory (~1,000 tokens)                                   ││
│  │ 3. Retrieved Memories (~500 tokens)                                 ││
│  │ 4. Conversation History (variable, up to ~120,000 tokens)           ││
│  │    ┌─────────────────────────────────────────────────────────────┐  ││
│  │    │ Compacted History     │ Recent Turns (uncompacted)          │  ││
│  │    │ (turns 1 to N-20)     │ (turns N-19 to N)                   │  ││
│  │    │ → Summarized          │ → Full detail                       │  ││
│  │    │ → In LTM              │ → In context                        │  ││
│  │    └─────────────────────────────────────────────────────────────┘  ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  When used_tokens > threshold (80%):                                     │
│    1. SPAWN_TASK: "Compact conversation history"                         │
│    2. Task summarizes turns 1 to N-20                                    │
│    3. Store summary in LTM                                               │
│    4. Replace old history with summary reference                         │
│    5. Keep turns N-19 to N in full                                       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Conversation History Management Process:**

```
1. Initialize Manager
   │
   ├─► Set max_tokens (default 128,000)
   ├─► Set compaction_threshold (default 80%)
   ├─► Set preserve_recent_turns (default 20)
   └─► Initialize empty history list

2. Add Turn (each iteration)
   │
   ├─► Create Turn object
   │   ├─► turn_id = sequential number
   │   ├─► thought = from agent
   │   ├─► action = from agent
   │   ├─► result = action result
   │   └─► tokens = estimated token count
   │
   ├─► Append to history
   │
   └─► Check if compaction needed

3. Check Compaction
   │
   ├─► Calculate total tokens:
   │   ├─► System prompt tokens
   │   ├─► Working memory tokens
   │   ├─► Retrieved memories tokens
   │   └─► Sum of all turn tokens
   │
   └─► If total > (max * threshold):
       └─► Trigger compaction

4. Trigger Compaction
   │
   ├─► Spawn compaction task
   │   ├─► task_id = new UUID
   │   ├─► description = "Compact history turns 1 to N-20"
   │   └─► agent_type = "reasoner"
   │
   └─► Task runs asynchronously

5. Compaction Task Execution
   │
   ├─► Get turns 1 to N-20
   ├─► Format turns into text
   ├─► Call LLM to summarize
   ├─► Store summary in LTM
   ├─► Update compacted_summary
   └─► Remove old turns from history

6. Build Context (for LLM calls)
   │
   ├─► Add system prompt
   ├─► If compacted_summary exists:
   │   └─► Add as "Previous conversation summary"
   ├─► Add recent turns (N-19 to N) in full
   └─► Return context list
```

**Turn Structure:**

| Field | Type | Description |
|-------|------|-------------|
| turn_id | integer | Sequential turn number |
| thought | string | Agent reasoning |
| action | string | Action taken |
| result | object | Action result |
| tokens | integer | Estimated token count |

**Context Building:**

```
┌─────────────────────────────────────────────────────────┐
│                  Final Context Structure                 │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. System Prompt                                       │
│     "You are an autonomous agent..."                    │
│                                                         │
│  2. Compacted Summary (if exists)                       │
│     "Previous conversation summary:                     │
│      Turns 1-180 covered database design, JWT auth..."  │
│                                                         │
│  3. Recent Turns (full detail)                          │
│     Turn 181: <thought>...</thought><action>...</action>│
│     Result: {...}                                       │
│     ...                                                 │
│     Turn 200: <thought>...</thought><action>...</action>│
│     Result: {...}                                       │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Configuration:**
```json
{
    "context": {
        "max_tokens": 128000,
        "compaction_threshold": 0.8,
        "preserve_recent_turns": 20,
        "compaction_agent_type": "reasoner"
    }
}
```

**Example Flow:**
```
Turn 1-180: Full conversation history
Context tokens: 95,000 / 128,000 (74%)
Action: Continue normal

Turn 181-200: More conversation
Context tokens: 105,000 / 128,000 (82%) ← Exceeded 80% threshold
Action: Trigger compaction

Compaction Task spawned:
- Summarizes turns 1-180
- Stores summary in LTM
- Removes turns 1-180 from context
- Keeps turns 181-200 in full

New context:
- Summary: "Previous turns covered database design, JWT auth..."
- Recent turns 181-200: Full detail
- New token count: ~25,000 / 128,000 (19%)
```

---

### 2.8.2 RAG Usage

**Usage Format:**

```
action: SEARCH_LONG_TERM
payload:
  query: "What database are we using?"
  top_k: 5
```

**Search Long-Term Memory Process:**

```
1. Receive Action
   │
   ├─► Extract query from payload
   ├─► Extract top_k (default 5)
   └─► Extract session_id (default: current session)

2. Generate Query Embedding
   │
   ├─► Call embedding model
   │   └─► Model: BAAI/bge-m3
   │
   └─► Get vector (1024 dimensions)

3. Semantic Search
   │
   ├─► Query database:
   │   ├─► SELECT id, content, tier, similarity
   │   ├─► FROM long_term_memories
   │   ├─► WHERE session_id = ?
   │   ├─► ORDER BY embedding <=> query_embedding
   │   └─► LIMIT top_k
   │
   └─► Get results sorted by similarity

4. Update Access Count
   │
   ├─► For each result:
   │   └─► Increment access_count by 1
   │   └─► Update last_accessed_at to now
   │
   └─► For retention tracking

5. Return Results
   │
   └─► ActionResult with memories list
```

**SQL Query:**

```
SELECT id, content, tier,
       1 - (embedding <=> query_embedding) AS similarity
FROM long_term_memories
WHERE session_id = ?
ORDER BY similarity DESC
LIMIT ?
```

**Result Structure:**

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Memory ID |
| content | text | Memory content |
| tier | string | Tier 1/2/3 |
| similarity | float | Cosine similarity (0-1) |
```

---

### 2.8.3 LTM Retention Policy

**Retention Policy:**
| Tier | Content | Access Threshold | Retention | Auto-Consolidate To |
|------|---------|------------------|-----------|---------------------|
| Tier 1 (Raw) | Full working memory entries | ≥3 accesses | 7 days | Tier 2 (Summary) |
| Tier 2 (Summary) | LLM summary of related entries | ≥5 accesses | 30 days | Tier 3 (Brief) |
| Tier 3 (Brief) | Highly condensed description | ≥10 accesses | 90 days | Deleted |

**Example Flow:**
```
Agent A (session S1) working memory:
- "User wants PostgreSQL database"
- "Auth system should use JWT"
- "API endpoints: /users, /auth, /data"

Agent B (session S1) working memory:
- "Frontend: React with TypeScript"
- "API endpoints: /users, /auth, /data"
- "Use Tailwind CSS for styling"

Consolidation (after 5 minutes):
1. Scan both agents' working memories
2. Identify frequent entries (accessed ≥3 times):
   - "API endpoints: /users, /auth, /data" (accessed by both agents)
3. Generate embeddings and store in Tier 1

After 7 days (if not accessed):
- LLM summarizes Tier 1 entries:
  "Project: REST API with PostgreSQL, JWT auth, React/TypeScript frontend,
   Tailwind CSS. Endpoints: /users, /auth, /data"
- Store in Tier 2, delete Tier 1

After 30 more days (if not accessed):
- LLM creates brief description:
  "REST API project (PostgreSQL, JWT, React)"
- Store in Tier 3, delete Tier 2

After 90 more days (if not accessed):
- Delete Tier 3 entry
```

---

### 2.8.4 Tool Calling Tools

**Design Principle:** Agents can call external tools via MCP or direct API integrations. All tool calls are tracked for billing and audit purposes.

**Tool Categories:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Tool Calling Architecture                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ AGENT (Thought-Action Loop)                                         ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                          │                                               │
│                          ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Tool Router                                                         ││
│  │ - Routes to MCP tools or direct tools                               ││
│  │ - Tracks token usage per call                                       ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                  │                           │                           │
│                  ▼                           ▼                           │
│  ┌─────────────────────────┐   ┌─────────────────────────┐               │
│  │ MCP Tools               │   │ Direct API Tools        │               │
│  │ (via MCP servers)       │   │ (built-in integrations) │               │
│  │ - Web search            │   │ - Token counter         │               │
│  │ - File operations       │   │ - Billing tracker       │               │
│  │ - Database queries      │   │ - JWT validator         │               │
│  │ - External APIs         │   │                         │               │
│  └─────────────────────────┘   └─────────────────────────┘               │
│                  │                           │                           │
│                  ▼                           ▼                           │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Token Tracker (per call)                                            ││
│  │ - Input tokens                                                      ││
│  │ - Output tokens                                                     ││
│  │ - Tool name                                                         ││
│  │ - Task ID (for aggregation)                                         ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Built-in Tool List:**

| Tool Category | Tool Name | Description |
|---------------|-----------|-------------|
| **Token/Billing** | count_tokens | Count input/output tokens for billing |
| | track_usage | Record token usage per task/session |
| | get_usage_summary | Get billing summary for session |
| **Authentication** | validate_jwt | Validate user JWT token |
| | get_user_claims | Extract user info from JWT |
| | check_membership | Check user membership level |
| **Search** | web_search | Search the web (via MCP) |
| | search_files | Search session files |
| | search_memory | Search long-term memory |
| **File Operations** | read_file | Read file from session storage |
| | write_file | Write file to session storage |
| | delete_file | Delete file from session storage |
| | list_files | List files in directory |
| **Code Execution** | run_python | Execute Python code in container |
| | run_shell | Execute shell command in container |
| | install_package | Install Python package in container |
| **Database** | query_db | Execute SQL query |
| | insert_record | Insert record into database |
| | update_record | Update record into database |
| **MCP Tools** | call_mcp_tool | Call any registered MCP tool |
| | list_mcp_tools | List available MCP tools |
| | add_mcp_server | Add new MCP server |

---

### 2.8.5 Token Tracking and Billing

**Design Principle:** Every token consumed is tracked per task and per session for billing and accounting purposes. User membership level (from JWT) determines priority and rates.

**Token Flow:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                      Token Tracking Architecture                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  LLM Call                       Tool Call                               │
│  ┌──────────────┐               ┌──────────────┐                        │
│  │ Input        │               │ Input        │                        │
│  │ Prompt       │               │ Parameters   │                        │
│  │ [tokens: N]  │               │ [tokens: N]  │                        │
│  └──────────────┘               └──────────────┘                        │
│         │                               │                                │
│         ▼                               ▼                                │
│  ┌──────────────┐               ┌──────────────┐                        │
│  │ LLM Response │               │ Tool Result  │                        │
│  │ [tokens: M]  │               │ [tokens: M]  │                        │
│  └──────────────┘               └──────────────┘                        │
│         │                               │                                │
│         └───────────────┬───────────────┘                                │
│                         ▼                                                │
│         ┌────────────────────────────────┐                               │
│         │  Token Tracker (per call)      │                               │
│         │  - task_id                     │                               │
│         │  - session_id                  │                               │
│         │  - operation_type              │                               │
│         │  - input_tokens                │                               │
│         │  - output_tokens               │                               │
│         │  - timestamp                   │                               │
│         │  - user_id (from JWT)          │                               │
│         └────────────────────────────────┘                               │
│                         │                                                │
│                         ▼                                                │
│         ┌────────────────────────────────┐                               │
│         │  Usage Aggregator              │                               │
│         │  - Per-task totals             │                               │
│         │  - Per-session totals          │                               │
│         │  - Per-user totals             │                               │
│         │  - Membership level            │                               │
│         └────────────────────────────────┘                               │
│                         │                                                │
│                         ▼                                                │
│         ┌────────────────────────────────┐                               │
│         │  Billing Engine                │                               │
│         │  - Apply membership rates      │                               │
│         │  - Calculate costs             │                               │
│         │  - Generate invoices           │                               │
│         └────────────────────────────────┘                               │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Usage Recording Schema:**

| Field | Type | Description |
|-------|------|-------------|
| usage_id | UUID | Unique usage record ID |
| task_id | UUID | Task that generated this usage |
| session_id | UUID | Session (user) context |
| user_id | string | User ID from JWT |
| operation_type | enum | LLM_CALL, TOOL_CALL, EMBEDDING |
| operation_name | string | Model name or tool name |
| input_tokens | integer | Tokens consumed in input |
| output_tokens | integer | Tokens consumed in output |
| total_tokens | integer | input + output |
| membership_level | string | User membership from JWT |
| cost_multiplier | float | Based on membership |
| effective_tokens | float | total * multiplier |
| timestamp | datetime | When usage occurred |

**Billing Tiers by Membership:**

| Membership | Priority | Cost Multiplier | Rate Limit |
|------------|----------|-----------------|------------|
| Free | Low | 1.0x | 10 tasks/hour |
| Basic | Normal | 0.9x | 50 tasks/hour |
| Pro | High | 0.7x | 200 tasks/hour |
| Enterprise | Critical | 0.5x | Unlimited |

---

### 2.8.6 JWT Authentication

**Design Principle:** External authentication system provides JWT tokens. JWT contains user identity, membership level, and permissions. All requests must include valid JWT.

**JWT Validation Flow:**

```
External Auth System          Catgirl Runtime
┌─────────────────┐           ┌─────────────────────────┐
│                 │           │                         │
│  ┌───────────┐  │           │  ┌───────────────────┐  │
│  │ User DB   │  │           │  │ API Gateway       │  │
│  │           │  │           │  │                   │  │
│  │ - Users   │  │           │  │ ┌─────────────┐   │  │
│  │ - Members │  │           │  │ │ JWT Validator│   │  │
│  │   Levels  │  │           │  │ └─────────────┘   │  │
│  └───────────┘  │           │  │         │         │  │
│         │       │           │  │         ▼         │  │
│  ┌───────────┐  │           │  │ ┌─────────────┐   │  │
│  │ JWT       │  │  Token    │  │ │ Task Queue  │   │  │
│  │ Generator │──│───────────│──│ │ Priority:   │   │  │
│  └───────────┘  │           │  │ │ Based on    │   │  │
│                 │           │  │ │ membership  │   │  │
│                 │           │  │ └─────────────┘   │  │
└─────────────────┘           │  └───────────────────┘  │
                              └─────────────────────────┘
```

**JWT Claims Structure:**

| Claim | Type | Description |
|-------|------|-------------|
| sub | string | User ID (subject) |
| email | string | User email |
| membership | enum | Free, Basic, Pro, Enterprise |
| session_id | UUID | Current session ID |
| exp | timestamp | Token expiration |
| iat | timestamp | Token issued at |
| permissions | list | Allowed actions |

---

### 2.8.7 On-Demand Agent Pool

**Design Principle:** Agents are spawned on-demand when tasks are enqueued (not pre-spawned). Agents are destroyed after completing their task. Pool has configurable maximum concurrent agents.

**On-Demand Agent Lifecycle:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    On-Demand Agent Lifecycle                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. Task Enqueued                                                        │
│     │                                                                    │
│     ▼                                                                    │
│  2. Check Agent Count                                                    │
│     │                                                                    │
│     ├─ If count < max --- Spawn new agent                               │
│     │                                                                    │
│     └─ If count >= max --- Queue task (wait for available agent)        │
│     │                                                                    │
│     ▼                                                                    │
│  3. Agent Spawned                                                        │
│     - Agent ID generated                                                 │
│     - Agent thread started                                               │
│     - Agent status = BUSY                                                │
│     │                                                                    │
│     ▼                                                                    │
│  4. Agent Picks Up Task                                                  │
│     - Task status = ASSIGNED                                             │
│     - Agent current_task = task_id                                       │
│     │                                                                    │
│     ▼                                                                    │
│  5. Agent Executes Task                                                  │
│     - Thought-action loop runs                                           │
│     - Token usage tracked                                                │
│     │                                                                    │
│     ▼                                                                    │
│  6. Task Completed                                                       │
│     - Result stored                                                      │
│     - Usage recorded                                                     │
│     │                                                                    │
│     ▼                                                                    │
│  7. Agent Destroyed                                                      │
│     - Agent thread stopped                                               │
│     - Agent removed from pool                                            │
│     - Count decremented                                                  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Agent State Machine:**

```
               ┌─────────────┐
               │     IDLE    │
               │ (available) │
               └──────┬──────┘
                      │
         Task assigned│
                      ▼
               ┌─────────────┐
               │     BUSY    │
               │ (executing) │
               └──────┬──────┘
                      │
         Task complete│
                      ▼
               ┌─────────────┐
               │ DESTROYING  │
               │ (cleanup)   │
               └──────┬──────┘
                      │
           Cleanup done│
                      ▼
               ┌─────────────┐
               │  REMOVED    │
               │ (from pool) │
               └─────────────┘
```

**Configuration:**

| Setting | Default | Description |
|---------|---------|-------------|
| max_agents | 50 | Maximum concurrent agents |
| spawn_timeout | 30s | Timeout for agent spawn |
| cleanup_timeout | 10s | Timeout for agent cleanup |
| task_wait_timeout | 5min | How long task waits for agent |

---

### 2.8.8 Membership-Based Task Priority

**Design Principle:** Task priority is determined solely by user membership level (from JWT), not by wait time. Higher membership = higher priority = faster execution.

**Priority Queue Structure:**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Membership-Based Priority Queue                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  PRIORITY      MEMBERSHIP        AGENT SELECTION                        │
│                                                                          │
│  ┌─────────┐  ┌──────────────┐   ┌─────────────────────────────────┐    │
│  │CRITICAL │  │  Enterprise  │   │  Agents pick from top first     │    │
│  └─────────┘  └──────────────┘   └─────────────────────────────────┘    │
│  ┌─────────┐  ┌──────────────┐                                          │
│  │   HIGH  │  │     Pro      │                                          │
│  └─────────┘  └──────────────┘                                          │
│  ┌─────────┐  ┌──────────────┐                                          │
│  │  NORMAL │  │    Basic     │                                          │
│  └─────────┘  └──────────────┘                                          │
│  ┌─────────┐  ┌──────────────┐                                          │
│  │   LOW   │  │    Free      │                                          │
│  └─────────┘  └──────────────┘                                          │
│                                                                          │
│  Note: Wait time does NOT affect priority                               │
│        Enterprise tasks always processed before Free tasks              │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Task Enqueue Flow:**

```
1. User submits task
   │
   ▼
2. Extract JWT from request
   │
   ▼
3. Decode JWT claims
   - membership: "Pro"
   - user_id: "user_123"
   │
   ▼
4. Set task priority
   - Pro → HIGH priority
   │
   ▼
5. Add to priority queue
   - Position based on priority only
   - NOT based on timestamp
   │
   ▼
6. Agents poll queue
   - Always pick highest priority first
```

---

## 3. Database Schema

### 3.1 Core Tables

**sessions:**
```sql
CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    telegram_user_id BIGINT NOT NULL,
    name VARCHAR(255),
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    orchestrator_state JSONB,
    metadata JSONB
);
```

**task_families:**
```sql
CREATE TABLE task_families (
    task_id UUID PRIMARY KEY,
    session_id UUID REFERENCES sessions(id),
    container_id VARCHAR(255),
    root_description TEXT NOT NULL,
    status VARCHAR(50) DEFAULT 'in_progress',
    max_depth_reached INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
```

**task_instances:**
```sql
CREATE TABLE task_instances (
    instance_id UUID PRIMARY KEY,
    task_id UUID REFERENCES task_families(task_id),
    session_id UUID REFERENCES sessions(id),
    owner_id VARCHAR(255) NOT NULL,
    depth INTEGER NOT NULL DEFAULT 0,
    description TEXT NOT NULL,
    agent_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    priority VARCHAR(20) DEFAULT 'normal',
    priority_score FLOAT DEFAULT 0,
    assigned_agent_id VARCHAR(255),
    parent_instance_id UUID REFERENCES task_instances(instance_id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    result JSONB,
    error TEXT,
    constraints JSONB,
    container_snapshot_id UUID
);
CREATE INDEX idx_instances_task ON task_instances(task_id);
CREATE INDEX idx_instances_session ON task_instances(session_id);
CREATE INDEX idx_instances_owner ON task_instances(owner_id);
CREATE INDEX idx_instances_status ON task_instances(status);
CREATE INDEX idx_instances_priority ON task_instances(priority_score DESC);
CREATE INDEX idx_instances_depth ON task_instances(depth);
```

**container_snapshots:**
```sql
CREATE TABLE container_snapshots (
    snapshot_id UUID PRIMARY KEY,
    task_id UUID REFERENCES task_families(task_id),
    instance_id UUID REFERENCES task_instances(instance_id),
    session_id UUID REFERENCES sessions(id),
    container_id VARCHAR(255) NOT NULL,
    image_id VARCHAR(255) NOT NULL,
    image_name VARCHAR(255),
    reason VARCHAR(50) NOT NULL,
    volumes JSONB,
    environment JSONB,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_snapshots_task ON container_snapshots(task_id);
CREATE INDEX idx_snapshots_session ON container_snapshots(session_id);
CREATE INDEX idx_snapshots_reason ON container_snapshots(reason);
CREATE INDEX idx_snapshots_expires ON container_snapshots(expires_at);
```

**agents:**
```sql
CREATE TABLE agents (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'idle',
    current_instance_id UUID REFERENCES task_instances(instance_id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ,
    tasks_completed INTEGER DEFAULT 0,
    metadata JSONB
);
-- Note: No session_id - agents are GLOBAL, not per-session
```

**working_memory:**
```sql
CREATE TABLE working_memory (
    agent_id VARCHAR(255) NOT NULL,  -- Each agent has its own working memory
    key VARCHAR(255) NOT NULL,
    value JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agent_id, key)
);
CREATE INDEX idx_working_memory_agent ON working_memory(agent_id);
```

**long_term_memories:**
```sql
CREATE TABLE long_term_memories (
    id UUID PRIMARY KEY,
    session_id UUID REFERENCES sessions(id),
    content TEXT,
    embedding VECTOR(1024),
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_memories_session ON long_term_memories(session_id);
CREATE INDEX idx_memories_embedding ON long_term_memories USING ivfflat(embedding cosine_ops);
```

**skills:**
```sql
CREATE TABLE skills (
    id UUID PRIMARY KEY,
    session_id UUID REFERENCES sessions(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    version VARCHAR(50) DEFAULT '1.0.0',
    definition JSONB,
    code TEXT,
    created_by_agent_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(session_id, name)
);
```

**mcp_servers:**
```sql
CREATE TABLE mcp_servers (
    id UUID PRIMARY KEY,
    session_id UUID REFERENCES sessions(id),
    name VARCHAR(255) NOT NULL,
    connection_type VARCHAR(50),
    connection_string TEXT,
    command VARCHAR(500),
    status VARCHAR(50) DEFAULT 'disconnected',
    tools JSONB,
    created_by_agent_id VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(session_id, name)
);
```

---

## 4. Global Queue Example

**Scenario:** Three sessions submitting tasks concurrently

```
Session #1 (User A): "Build a REST API"
Session #2 (User B): "Analyze this dataset"
Session #3 (User C): "Write documentation"

Global Task Queue (priority-sorted):
┌─────────────────────────────────────────────────────────────┐
│ Priority │ Session │ Task              │ Depth │ Type      │
├─────────────────────────────────────────────────────────────┤
│   4.0    │   S1    │ "Build REST API"  │   0   │ GP        │
│   3.5    │   S2    │ "Analyze dataset" │   0   │ Reasoner  │
│   3.0    │   S3    │ "Write docs"      │   0   │ GP        │
│   2.0    │   S1    │ "Design DB"       │   1   │ Reasoner  │
│   2.0    │   S1    │ "Implement auth"  │   1   │ GP        │
│   1.0    │   S2    │ "Clean data"      │   1   │ GP        │
└─────────────────────────────────────────────────────────────┘

Global Agent Pool:
- Agent1 (GP) → picks "Build REST API" (S1)
- Agent2 (GP) → picks "Write docs" (S3)
- Agent3 (Reasoner) → picks "Analyze dataset" (S2)

When Agent1 spawns sub-tasks:
- Sub-tasks enter SAME global queue
- Sub-tasks compete with ALL other tasks
- Highest priority tasks get picked first
```

---

## 5. Container Snapshot Example

**Scenario:** Task completes, snapshot created for later recall

```
Task instance A6 (depth=3) completes successfully
│
▼
Runtime triggers snapshot creation
│
▼
Snapshot created:
{
    "snapshot_id": "snap_abc123",
    "task_id": "UUID_A",
    "instance_id": "A6",
    "session_id": "S1",
    "reason": "COMPLETED",
    "image": {
        "name": "snapshot_A6_20260321_123456",
        "size_bytes": 125000000,
        "layers": ["sha256:abc...", "sha256:def..."]
    },
    "volumes": {
        "/workspace": {
            "files": ["src/main.py", "tests/test_main.py", "output.txt"],
            "total_size_bytes": 50000
        }
    },
    "metadata": {
        "agent_id": "Agent_GP3",
        "execution_time_seconds": 45.2,
        "memory_used_bytes": 52428800,
        "cpu_time_seconds": 30.5
    },
    "expires_at": "2026-03-28T12:34:56Z"  # 7 days from now
}
│
▼
Snapshot stored in container_snapshots table
│
▼
Running container released (freed)
│
▼
Snapshot available for:
- Debug inspection
- Resume execution (if needed)
- Audit trail
│
▼
After 7 days: Snapshot auto-deleted (unless extended)
```

---

## 6. Task-Owner Communication Channel

**Design Principle:** Simple FIFO queue of text messages between task and owner. No fixed message types - semantics are determined by content.

**Channel Properties:**
```
TaskOwnerChannel {
    task_instance_id: UUID
    owner_id: string
    messages: FIFOQueue<TextMessage>
    created_at: timestamp
    last_activity: timestamp
}

TextMessage {
    id: UUID
    from_id: string (sender: task_instance_id or owner_id)
    to_id: string (receiver: owner_id or task_instance_id)
    content: string (arbitrary text)
    timestamp: timestamp
    read: boolean
}
```

**Operations:**
```
send_message(channel_id, from_id, to_id, content) -> message_id:
    message = TextMessage(
        id = generate_uuid(),
        from_id = from_id,
        to_id = to_id,
        content = content,
        timestamp = now(),
        read = false
    )
    channel.messages.enqueue(message)
    notify_receiver(to_id, message)
    return message.id

read_messages(channel_id, receiver_id) -> list[TextMessage]:
    messages = []
    for msg in channel.messages:
        if msg.to_id == receiver_id and not msg.read:
            msg.read = true
            messages.append(msg)
    return messages

get_unread_count(channel_id, receiver_id) -> integer:
    count = 0
    for msg in channel.messages:
        if msg.to_id == receiver_id and not msg.read:
            count++
    return count
```

**Communication Patterns (by convention, not enforced):**
```
# Task reports completion
task → owner: "Task completed successfully. Result: [summary]"

# Task reports failure
task → owner: "Task failed: [error description]. Partial result: [details]"

# Task requests clarification
task → owner: "Need clarification: [question about specific aspect]"
owner → task: "Clarification: [answer]"

# Task requests resources
task → owner: "Request: [resource needed, e.g., more memory, API access]"
owner → task: "Granted: [resource details]"

# Owner interrupts task
owner → task: "Interrupt: [reason, e.g., cancel, pause, change direction]"
task → owner: "Acknowledged: [action taken]"

# Task sends progress update
task → owner: "Progress: [percentage]% - [current status]"
```

**Channel Lifecycle:**
```
1. Task instance created
   │
   ▼
2. Communication channel created
   - channel_id = task_instance_id (1:1 mapping)
   - owner_id = task.owner_id
   │
   ▼
3. Task and owner exchange messages
   - Messages stored in FIFO order
   - Unread messages tracked per receiver
   │
   ▼
4. Task exits (any reason)
   │
   ▼
5. Channel preserved for history
   - Messages retained for audit/resume
   - Read/unread status preserved
   │
   ▼
6. Channel cleaned up with snapshot retention
```

**Database Schema:**
```sql
CREATE TABLE task_owner_channels (
    channel_id UUID PRIMARY KEY,
    task_instance_id UUID REFERENCES task_instances(instance_id),
    owner_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_activity TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_channels_task ON task_owner_channels(task_instance_id);
CREATE INDEX idx_channels_owner ON task_owner_channels(owner_id);

CREATE TABLE task_messages (
    message_id UUID PRIMARY KEY,
    channel_id UUID REFERENCES task_owner_channels(channel_id),
    from_id VARCHAR(255) NOT NULL,
    to_id VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    read BOOLEAN DEFAULT FALSE
);
CREATE INDEX idx_messages_channel ON task_messages(channel_id);
CREATE INDEX idx_messages_unread ON task_messages(to_id, read);
CREATE INDEX idx_messages_timestamp ON task_messages(timestamp);
```

**Example Exchange:**
```
Task A6 (instance_id=A6, owner_id=S1:Agent_GP2)

Channel: A6 ↔ S1:Agent_GP2

[A6 → S1:Agent_GP2] "Starting SQL migration implementation"
[A6 → S1:Agent_GP2] "Progress: 25% - Analyzed existing schema"
[S1:Agent_GP2 → A6] "Acknowledge. Focus on PostgreSQL compatibility."
[A6 → S1:Agent_GP2] "Progress: 60% - Migration scripts generated"
[A6 → S1:Agent_GP2] "Question: Should I include rollback scripts?"
[S1:Agent_GP2 → A6] "Yes, include rollback scripts for each migration."
[A6 → S1:Agent_GP2] "Progress: 90% - Rollback scripts added"
[A6 → S1:Agent_GP2] "Task completed. Files: migrations/001_initial.sql, rollbacks/001_initial.sql"
```

**Cross-Session Consideration:**
Since agents are global, owner_id must be globally unique (e.g., `session_id:agent_id` format).

---

## 7. Configuration

### 7.1 Global Configuration

```json
{
    "global": {
        "max_task_depth": 3,
        "max_queue_size": 1000,
        "container_cleanup_delay_seconds": 300,
        "snapshot_enabled": true
    }
}
```

### 7.2 Agent Pool Configuration

```json
{
    "agent_pool": {
        "min_agents": 5,
        "max_agents": 50,
        "gp_agent_ratio": 0.8,
        "idle_timeout_seconds": 300,
        "max_tasks_per_agent": 100
    }
}
```

### 7.3 Snapshot Configuration

```json
{
    "snapshots": {
        "enabled": true,
        "retention": {
            "COMPLETED": "7d",
            "FAILED": "30d",
            "EXITED": "7d",
            "INTERRUPTED": "14d"
        },
        "storage_path": "/var/catgirl/snapshots",
        "max_storage_bytes": 10737418240
    }
}
```

---

## 8. Glossary

| Term | Definition |
|------|------------|
| **Session** | Isolated environment for one user (per-session orchestrator + memory) |
| **Main Orchestrator** | Per-session coordinator with LIMITED capabilities |
| **Global Task Queue** | Single priority queue for ALL tasks across ALL sessions |
| **Global Agent Pool** | Shared worker agents available to ANY session |
| **Task Family** | Group of task instances sharing same task_id |
| **task_id** | Identifier for task family (shared by parent and sub-tasks) |
| **instance_id** | Unique identifier for each task instance |
| **owner_id** | Agent ID for receiving results/communications |
| **Container Snapshot** | Saved state of container on task exit (for recall) |
| **Depth** | Task instance level (0 = root, increases for sub-tasks) |

---

## 9. Implementation Checklist - Major Components

This checklist lists the **major architectural components** required to implement the system. It intentionally omits minor utilities and obvious low-level helpers.

### 9.1 Runtime Core

| Component | Description |
|-----------|-------------|
| Runtime Coordinator | Starts and stops the whole system, wires together HTTP server, task queue, agent pool, storage, and background services. |
| Configuration System | Loads runtime configuration, validates it, and exposes settings for models, queues, snapshots, auth, and billing. |
| Observability Layer | Structured logs, health reporting, metrics, and operational diagnostics across sessions, tasks, agents, and containers. |

### 9.2 Authentication and Identity

| Component | Description |
|-----------|-------------|
| JWT Authentication Gateway | Validates JWTs issued by the external auth provider and rejects invalid or expired tokens before task creation. |
| Identity & Membership Resolver | Extracts user identity, membership level, permissions, and session context from JWT claims for use in scheduling, billing, and authorization. |
| Authorization Policy | Enforces which capabilities are available to the main orchestrator, worker agents, and API clients based on role and membership. |

### 9.3 Session Layer

| Component | Description |
|-----------|-------------|
| Session Manager | Creates, loads, and stops user sessions. Maintains one orchestrator, one long-term memory space, one MCP space, and one skill space per session. |
| Session State Store | Persists session metadata, orchestrator state, session settings, and recovery checkpoints so sessions can survive restarts. |
| Session Resource Registry | Keeps track of the session-scoped resources that all workers in the same session may access, including long-term memory, files, MCP servers, and skills. |

### 9.4 Main Orchestrator

| Component | Description |
|-----------|-------------|
| Main Orchestrator Agent | The single coordinator agent per session. It receives user goals, reasons about decomposition, spawns root tasks, monitors progress, and aggregates results. |
| Orchestrator Context Builder | Builds the limited context available to the orchestrator: its own working memory, session long-term memory retrieval, conversation history, and task summaries. |
| Orchestrator Response Manager | Decides what to send back to the user, when to wait for tasks, and when to ask for clarification or provide progress updates. |

### 9.5 Global Task System

| Component | Description |
|-----------|-------------|
| Global Task Queue | Holds task instances from all sessions in a single global queue. Supports dequeue by priority and agent type, while preserving session isolation through task metadata. |
| Task Family Manager | Manages task families identified by `task_id`, including root tasks and all sub-task instances belonging to the same task tree. |
| Task Instance Manager | Creates, updates, and tracks individual task instances (`instance_id`) with depth, owner, status, result, and container snapshot references. |
| Task Priority Engine | Calculates task priority exclusively from user membership level in JWT claims, not from queue waiting time. |
| Depth Control Manager | Enforces maximum task tree depth and blocks sub-task spawning once the configured limit is reached. |

### 9.6 Global Agent Pool

| Component | Description |
|-----------|-------------|
| Agent Pool Manager | Maintains the global pool of worker agents. Spawns workers on demand when tasks are enqueued, up to a configured maximum. |
| Worker Lifecycle Manager | Creates a worker when needed, assigns one task to it, and destroys the worker after task completion or failure. |
| Agent Assignment Engine | Matches queued task instances to available worker type (general-purpose or reasoner) and handles fallback rules if exact matches are unavailable. |
| Worker State Tracker | Tracks worker status, current task, last activity, failures, and completed workload for audit and operational visibility. |

### 9.7 Worker Agent Types

| Component | Description |
|-----------|-------------|
| General-Purpose Worker Agent | Fast, lower-cost worker used for routine execution, file manipulation, tool calls, code execution, and straightforward task decomposition. |
| Reasoner Worker Agent | Slower, deeper-reasoning worker used for planning, debugging, summarization, analysis, and compaction tasks. |
| Worker Capability Boundary | Defines the full set of actions available to workers, including sub-task spawning, code execution, MCP usage, skill creation, and skill execution. |

### 9.8 Thought-Action Execution

| Component | Description |
|-----------|-------------|
| Thought-Action Loop Engine | Runs the iterative think → act → observe → continue cycle for both orchestrator and worker agents. |
| Action Parser | Converts raw model output into structured actions and validates that the action is allowed for the current agent type. |
| Action Executor | Dispatches actions to the correct subsystem (task system, memory, container, tools, skills, billing, communication). |
| Context Window Manager | Tracks context growth, injects retrieved memories, and triggers history compaction when the configured threshold is exceeded. |

### 9.9 Working Memory / Context

| Component | Description |
|-----------|-------------|
| Per-Agent Working Memory Store | Private short-term memory for each agent. Stores local context, intermediate reasoning artifacts, tool results, and temporary task state. |
| Working Memory Access Policy | Ensures working memory is only readable and writable by the owning agent unless information is explicitly shared through task results or long-term memory consolidation. |
| Working Memory Scanner | Supplies the consolidation system with candidate short-term memories that are frequently used and worth promoting into long-term memory. |

### 9.10 Long-Term Memory and RAG

| Component | Description |
|-----------|-------------|
| Long-Term Memory Store | Shared, read-only memory space per session that holds promoted memories, summaries, and archival descriptions. |
| Embedding Service | Generates vector embeddings for memory entries and queries so semantic retrieval is possible. |
| Memory Consolidation Service | Periodically scans all agent working memories in a session, promotes important entries, and manages multi-tier summarization over time. |
| Retrieval Service | Performs semantic search over long-term memory and automatically injects relevant retrieved context before each LLM call. |
| Memory Retention Manager | Applies the tiered retention policy for raw entries, summaries, and brief descriptions, including expiration and archival cleanup. |

### 9.11 Conversation History Management

| Component | Description |
|-----------|-------------|
| Conversation History Store | Keeps the full turn-by-turn conversation history for the session, including thoughts, actions, and results. |
| History Compaction Manager | Detects when the context window crosses the configured threshold and spawns a compaction task that summarizes older turns while preserving the most recent turns in full. |
| Compacted History Injector | Inserts the compacted summary into future contexts so prior history remains available in condensed form. |

### 9.12 Tool Calling Layer

| Component | Description |
|-----------|-------------|
| Tool Router | Unified routing layer for all tool invocations, whether they are MCP-backed or runtime-native tools. |
| Built-in Tool Set | Native tools for token counting, usage reporting, auth inspection, file operations, code execution coordination, and database operations where appropriate. |
| MCP Tool Adapter | Normalizes calls to external MCP tools so they behave consistently with built-in tools. |
| Tool Usage Auditor | Records each tool call, its inputs, outputs, timing, and association with task/session/user for debugging and billing. |

### 9.13 MCP Capability System

| Component | Description |
|-----------|-------------|
| Session MCP Client | Maintains the set of MCP servers connected for one session and exposes only those tools to that session's agents. |
| MCP Server Registry | Persists MCP server definitions, connection details, discovered tools, and connection state on a per-session basis. |
| MCP Discovery and Invocation Layer | Discovers tools from MCP servers and executes them safely with session isolation. |
| Autonomous MCP Provisioning | Allows worker agents to add MCP servers during execution when they need new capabilities. |

### 9.14 Skill System

| Component | Description |
|-----------|-------------|
| Skill Registry | Stores all skills available to a session, including metadata, prompt template, optional code, and versioning. |
| Skill Parser | Interprets the portable SKILL definition format and validates required sections. |
| Skill Execution Engine | Executes skills either as pure prompt templates or as prompt + code workflows. |
| Autonomous Skill Creation | Allows worker agents to create new reusable skills from repeated patterns discovered during task execution. |

### 9.15 Container Execution and Snapshots

| Component | Description |
|-----------|-------------|
| Container Manager | Creates, starts, stops, and removes task-family containers. Ensures all task instances with the same `task_id` resolve to the same container. |
| Container Execution Engine | Executes code and shell commands inside the task-family container with resource limits and timeout handling. |
| Container Snapshot Manager | Creates a snapshot whenever a task exits—completed, failed, interrupted, or otherwise terminated. |
| Snapshot Recall System | Restores a new runnable container from a stored snapshot for debugging, auditing, or continuation. |
| Snapshot Retention Manager | Applies retention rules to snapshots and removes expired images and metadata. |

### 9.16 Task-Owner Communication

| Component | Description |
|-----------|-------------|
| Task-Owner Channel | FIFO text-only communication channel between a task instance and its owner. |
| Message Persistence Layer | Stores all channel messages, unread state, timestamps, and audit history. |
| Owner Notification Manager | Delivers completion notices, clarification requests, progress updates, interruptions, and resource requests to the owner agent. |

### 9.17 Token Accounting and Billing

| Component | Description |
|-----------|-------------|
| Token Usage Recorder | Captures input and output tokens for every LLM call, embedding request, and tool call. |
| Per-Task Usage Aggregator | Maintains token totals and derived cost per task family and per task instance. |
| Per-Session Usage Aggregator | Maintains token totals and derived cost across the whole user session. |
| Billing Engine | Applies membership-based pricing or multipliers to raw token usage and produces billable usage records. |
| Usage Reporting Interface | Provides summaries for users, administrators, and automated accounting systems. |

### 9.18 Telegram Interface

| Component | Description |
|-----------|-------------|
| Telegram Gateway | Receives inbound Telegram updates, validates the user context, and routes user input into the correct session orchestrator. |
| Telegram Bot Client | Sends outbound messages, status updates, task results, and file attachments back to users. |
| Telegram User Registry | Tracks Telegram user identity, session association, ban state, and recent activity. |

### 9.19 API and Administration Surface

| Component | Description |
|-----------|-------------|
| API Server | Exposes operational endpoints for health, metrics, sessions, tasks, agents, usage, and snapshots. |
| Authentication Middleware | Protects administrative routes using JWT validation and permission checks. |
| Operational Routes | Session management, task inspection, worker inspection, usage/billing inspection, snapshot browsing, and system metrics. |

### 9.20 Persistence Layer

| Component | Description |
|-----------|-------------|
| Database Access Layer | Manages connections, transactions, pooling, and query execution against the relational/vector database. |
| Schema Migration System | Creates and upgrades all required tables, indexes, and extensions. |
| Repository Layer | Encapsulates storage operations for sessions, tasks, memories, skills, MCP servers, usage records, and snapshots. |

### 9.21 Background Services

| Component | Description |
|-----------|-------------|
| Long-Term Memory Consolidation Worker | Promotes frequently used working-memory items into long-term memory and performs summarization across tiers. |
| Conversation Compaction Worker | Executes history compaction tasks when context growth exceeds the configured threshold. |
| Snapshot Cleanup Worker | Removes expired snapshots and associated images according to retention policy. |
| Health and Metrics Monitor | Continuously measures queue depth, active agents, token consumption, snapshot storage, and component health. |

---

## Version History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-03-21 | Initial architecture |
| 2.0.0 | 2026-03-21 | Task-based architecture |
| 3.0.0 | 2026-03-21 | Limited orchestrator + depth-aware task tree |
| 3.1.0 | 2026-03-21 | Per-task Docker containers |
| 3.2.0 | 2026-03-21 | task_id shared by family, instance_id unique |
| 4.0.0 | 2026-03-21 | Global queue + global agent pool + container snapshots |
| 4.1.0 | 2026-03-22 | Per-agent working memory + auto LTM consolidation |
| 4.2.0 | 2026-03-22 | Per-session MCP client + implementation checklist |
| 4.3.0 | 2026-03-22 | Simplified RAG (search-only, no re-ranking) |
| 4.4.0 | 2026-03-22 | Tool calling, JWT auth, token tracking, on-demand agents |

---

## Document Statistics

| Metric | Value |
|--------|-------|
| Total Sections | 10 |
| Components to Implement | ~95 |
| Database Tables | 14 |
| Action Types | 12 |
| Built-in Tools | 20+ |
| JWT Claims | 7 |
| Billing Tiers | 4 |
| Configuration Options | ~25 |
