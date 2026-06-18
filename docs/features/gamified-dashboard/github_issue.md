# [RFC/Feature Request] 1Agents Hangar: Gamifying Project Management with AI Employee & Hangar Pipeline (V3 Design)

## Description / Background

1Agents is designed as a self-hosted, agentic collaborative workbench for solo developers and small teams to orchestrate tasks across heterogeneous nodes (Mac/Linux/Windows). 

Instead of traditional, dry, information-heavy project boards (Jira, GitHub Boards), we propose a **gamified dashboard (1Agents Hangar)** combining **Kairosoft-style simulation management (e.g. *Game Dev Story*)** and **Factorio-style automation loops**.

By mapping real-world engineering constraints (API cost, rate limits, prompt templates, and release lifecycles) to in-game values and animations, we aim to transform development into a highly rewarding game loop.

---

## Proposed Mechanics

### 1. The AI Employee Card & "Shadow Clones"
*   **Basic Employees**: Raw LLM configurations (e.g., standard Claude 3.5, GPT-4o). They carry no special system prompts or custom tools and are limited only by their raw API rate-limits.
*   **Specialist Employees**: Created by capturing **"best practices"** from successful chats. Users can package a successful Session's state (model base + ACP framework + specific versions of loaded Skills/Tools + system prompt/persona) as a "Specialist Card" template. 
*   **Infinite Cloning**: Unlike physical employees in traditional games, an AI employee card can be cloned and deployed to run concurrently across multiple tasks.
*   **Global Stamina Pool**: Clones of the same configuration share a global stamina pool (mapped to the LLM's rate limit, e.g., 50 requests/3h). When the pool drains, all active clones fall asleep on the screen, requiring time to cool down.

### 2. Effort Level Integration
*   **Workbench UI Slider**: Users adjust the reasoning effort level directly in the chat workbench (Low/Middle/High).
    *   *Low*: Fast, cheap, less stamina drain, but more prone to bugs (decreased stability).
    *   *High*: Long reasoning mode (Thinking Tokens enabled), slow, high token cost, drains stamina fast, but handles complex logic with high stability.
*   **Dashboard Visualizer**: The dashboard reads `sessions.effort_level` and renders varying LED spinning colors (Green = Low, Blue = Middle, Orange/Gold = High) on the workbench card along with varying frame animation speeds for the typing sprite.

### 3. "Building in Public" (Shipping Day)
*   Completed projects (all tasks finished) reach the launchpad. Clicking launch fires a rocket, triggering a **public release mockup page**.
*   **Public Metrics**: Based on the code quality, loaded skill versions, and defects, it simulates release metrics:
    *   *Views & Stars/Likes*: Simulating traction on GitHub, X, or Reddit.
    *   *Beta Feedback*: Simulates user feedback.
*   **Prestige**: Higher traction increases the company's **Reputation**, which can be spent to import/unlock higher-tier skill configurations or licensing CLI tools from the Talent Market.

### 4. Factorio-Style Achievements
An automation board logs indicators of scale:
*   *Max Throughput*: Cumulative successfully automated tasks.
*   *Automation Wonders*: Longest continuous sequence of tasks automatically triggered and resolved without human intervention (0 user replies on the issue timeline).
*   *Parallel Load limit*: Max concurrent active sessions within the hangar.

---

## Schema Changes (SQLite)

We propose the following extensions to `meta.db`:

```sql
-- 1. Track reasoning depth in session history
ALTER TABLE sessions ADD COLUMN effort_level TEXT NOT NULL DEFAULT 'middle';

-- 2. Store employee cards & specialists templates
CREATE TABLE employees (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    kind           TEXT NOT NULL DEFAULT 'basic', -- basic | specialist
    model_type     TEXT NOT NULL,
    framework      TEXT NOT NULL,
    skills_json    TEXT NOT NULL,              -- JSON list of skills + versions
    system_prompt  TEXT,                       -- Prompt snapshot for specialists
    persona        TEXT DEFAULT 'normal',      -- Custom dialogue ID
    rating_good    INTEGER DEFAULT 0,
    rating_normal  INTEGER DEFAULT 0,
    rating_poor    INTEGER DEFAULT 0,
    stamina        INTEGER DEFAULT 100,
    created_at     TEXT NOT NULL
);

-- 3. Match employee contributions to task history
CREATE TABLE employee_history (
    employee_id    TEXT NOT NULL REFERENCES employees(id),
    task_id        TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    performance    TEXT,                       -- excellent | normal | poor
    completed_at   TEXT NOT NULL,
    PRIMARY KEY (employee_id, task_id)
);

-- 4. Track public release metrics
CREATE TABLE project_public_metrics (
    project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    views          INTEGER DEFAULT 0,
    stars          INTEGER DEFAULT 0,
    phase          TEXT NOT NULL DEFAULT 'beta', -- alpha | beta | stable
    last_shipped   TEXT NOT NULL
);
```

---

## Feedback & Discussion

We'd love to hear the community's thoughts on:
1. What other stats or indicators should an AI employee carry?
2. Should we implement active "events" like server outages, model pricing fluctuations, or merge conflicts?
