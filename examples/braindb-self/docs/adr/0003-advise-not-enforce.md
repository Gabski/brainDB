# ADR 0003: purpose admission defaults to advise

- Status: accepted, 0.0.2
- Supersedes: admission enforcing by default
- Concepts: purpose, agentmem

## Context

Every brain declares a purpose, and every incoming item is scored against it. With `enforce`, an item scoring below
the accept threshold is dropped at the door. The score is a similarity number against a prose description. When it
is wrong, the knowledge is gone and nobody knows it was ever offered.

## Decision

`advise` is the default: the score and its reasons are recorded, everything is stored. `enforce` is opt-in, and
every shipped example uses `advise`.

## Consequences

- The cost moves from "invisible loss" to "visible noise", which is the direction a memory system should fail in.
- Before turning `enforce` on, a maintainer can see exactly what it would have dropped. The evidence is not yet
  easy to read; a purpose-score column in the Browse tab is the open idea.
- Agents receive the purpose text with their context, so the filter is also a prompt: it tells the agent what this
  brain is for.
