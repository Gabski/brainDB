# ADR 0002: decay must not delete knowledge

- Status: accepted, 0.0.2
- Supersedes: the 0.0.1 policy where every kind faded to zero
- Concepts: decay, storage

## Context

In 0.0.1 the weight curve was a plain half-life: `w = base * 0.5^(age / half-life)`. A decision taken a year ago
weighed almost nothing, so it fell out of every context slice. Nothing had deleted it, and yet it was gone.

## Decision

Every kind carries a **floor**, the share of its base weight that age can never take away:

```
w = base * (floor + (1 - floor) * 0.5^(age / half-life))
```

Durable kinds (decision 0.80, fact and record 0.70, file 0.60, chunk 0.55) rank lower with age but never leave the
graph. Attention-shaped kinds keep floor 0 and really do go away: task 14 d, question 7 d, note 7 d.

Only `SUPERSEDE` and `FORGET` remove knowledge.

## Consequences

- Existing brains keep their content, but weights, and therefore ranking and context, change on the next load.
- `braindb.yaml` can restore the old policy per kind with `{halfLife: 30, floor: 0}`, and `LegacyDecay()` restores
  it wholesale from Go.
- The parity fixture against the JavaScript prototype pins `LegacyDecay()`, because parity is about the algorithms,
  not about which half-life ships today.
