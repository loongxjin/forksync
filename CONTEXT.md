# ForkSync

ForkSync keeps forked GitHub repositories synchronized with their upstreams, and uses external coding agents to reconcile conflicts that arise during sync.

## Language

**Repo**:
A forked GitHub repository under ForkSync's management.
_Avoid_: repository (too generic), project.

**Upstream**:
The parent repository a Repo tracks and pulls changes from.

**Sync**:
One execution of the pull-and-reconcile pipeline for a Repo against its Upstream.
_Avoid_: update, pull, merge (those are steps within a Sync).

**Workflow**:
The ordered, state-tracked run of a Sync — fetch, merge, resolve, commit — expressed as a sequence of steps. The Workflow owns its own transition rules.
_Avoid_: pipeline, process.

**Conflict**:
A file that diverges between a Repo and its Upstream during a Sync's merge step and must be reconciled before the Sync can complete.

**Resolve**:
The act of reconciling Conflicts. In ForkSync this is done by an Agent.
_Avoid_: fix, patch.

**Conflict Resolver**:
The Module that performs Resolve: drives an Agent over the Conflicts, verifies Conflict markers are gone, and stages the result. It does not commit.
_Avoid_: merge driver, resolver service.

**ResolveOutcome**:
The result of a Conflict Resolver run: either Resolved (tree staged, with diff and summary) or Failed. Committing or pausing for confirmation is the caller's decision, not part of the outcome.

**Resolve Commit**:
The act of committing a Resolved tree. It is a separate action that _follows_ a Resolve — never part of Resolve itself. Two flavors: the user-driven confirmation commit (`Accept`, manual, produces a Manual-resolved message), and the auto commit (the scheduler's auto-resolve arm, no user review). Both share a commit pipeline but differ in trigger and history recording.
_Avoid_: folding this into Resolve; calling the `Resolver` type a "resolver" as if committing were its job. The `Resolver` hosts both the no-commit Resolve core and the `Accept` confirmation commit — these are two distinct responsibilities living on one type.

**Agent**:
An external coding agent (Claude, OpenCode, Codex) invoked to Resolve conflicts for a Repo.
_Avoid_: bot, assistant.

**Session**:
A conversation with an Agent, scoped to a Repo, reused across Syncs while active.

**History**:
The append-only record of Sync outcomes for a Repo.

## Relationships

- A **Repo** tracks exactly one **Upstream** and is advanced by repeated **Sync**s.
- A **Sync** runs as one **Workflow**.
- A **Workflow** produces zero or more **Conflict**s during its merge step.
- **Conflict**s are reconciled by the **Conflict Resolver**, which returns one **ResolveOutcome**.
- A **ResolveOutcome** of Resolved leads to either a **Resolve Commit** (auto, by the scheduler) or a paused **Workflow** awaiting the user's confirmation (which, when given, is a **Resolve Commit** of the `Accept` flavor).
- Each completed **Sync** appends one record to **History**.

## Example dialogue

> **Dev:** "When a Sync hits Conflicts, who commits the resolved tree?"
> **Domain expert:** "Not the Conflict Resolver — it only Resolves, verifies, and stages. The caller decides: the scheduler auto-commits, the interactive resolve flow pauses the Workflow for confirmation."

> **Dev:** "So what does the Workflow do when the Resolver returns Resolved?"
> **Domain expert:** "It transitions on the event — OnAgentResolved — and then either OnCommitted or stays paused. The Workflow owns those transitions; callers only emit events."

## Flagged ambiguities

- "resolve" is used both as the verb (the act) and informally for the resolve endpoint. Resolved: **Resolve** is the domain act; the HTTP endpoint is just a trigger of it.
- "Workflow" previously referred to both the in-memory step list and the package that holds it. Resolved: **Workflow** is the run (the state machine); the package is its implementation.
