# Agent playbook: building a knowledge base an agent can actually use

This is the practice distilled from running brainDB on itself. It is written for the person who has a pile of
documents, a model with a context window, and the suspicion that dumping the first into the second is not working.

## 1. Context is a slice, not a dump

A brain holds everything. A turn sees the **lit slice**: start at a focus, spread activation to a depth, keep what
stays above a threshold. The slice carries weights, activation, depth and the path the signal took, so the model
can tell central knowledge from a neighbour two hops away.

Rules that follow from the model:

- **Set the focus to the question, not to the project.** A focus of three ids answers one question well; a focus of
  twelve lights half the brain and costs four times the tokens for no extra answer.
- **Start at depth 1.** Widen only when the answer is not in the slice. Depth 3 almost never adds an answer; it adds
  a bill.
- **Raise the threshold on a dense graph.** This brain, with 72 hand-written links over 95 memories, lights 47% of
  itself at threshold 0.2 and 23% at 0.3. The same focus, the same depth, half the tokens.
- **Read the status line.** `lit 31/135 memories + 7 concepts, ~2254 tok` is the number to tune against, every turn.

## 2. Kinds carry policy, not decoration

A kind sets how long knowledge stays interesting: half-life, floor, base weight. The split that matters is between
**durable** and **attention-shaped** kinds.

| group | kinds | behaviour |
|---|---|---|
| durable | decision, fact, record, file, chunk | floor 0.55 to 0.80; rank lower with age, never leave the graph |
| fading | summary, idea | floor 0.25 to 0.40; go stale, stay findable |
| attention-shaped | task, question, note | floor 0; really do go away |

Write a project's standing knowledge as `decision` and `fact`. Write the state of play as `task` and `question`. A
task that still matters in three weeks either gets done or gets rewritten as a decision; that is the system working,
not a leak.

## 3. Supersede, never delete

The old version is the record of why the new one exists. `SUPERSEDE` keeps it at 30% weight, out of the default
slice and one flag away from being read. In this brain: enforce became advise, decay-to-zero became decay floors,
a hand-rolled storage engine became SQLite. Each pair is one query away from explaining itself.

## 4. Connectors carry the joins

Anchors, hubs, aliases and bridges hold no knowledge and cost no tokens, but activation routes through them. One
`hub` over the eleven memories about the paraphrase collapse means a question about retrieval quality reaches the
measurement, the cause, the open question and the task, without eleven hand-written edges between them.

Use an `alias` for the words your sources disagree on (ARR / annual contract value; ANN / vector index / kNN).

## 5. Judge the retrieval before trusting it

The uncomfortable finding from brainDB's own measurement: with the offline `hash-256` engine, queries that share
only *meaning* with their target return nothing relevant 12 to 14 times out of 16. Graph activation cannot rescue a
lexical seed, because it can only spread from what similarity found.

So:

- write a gold set of real questions with the ids that answer them;
- split it into **lexical** and **paraphrase** buckets, because the average over both hides the failure;
- run `braindb eval` after every change to kinds, links or engine;
- record the engine, dimension, node count, k and date next to every number. A number without its engine is not a
  result.

If your agents phrase questions in a user's own words, a real embedding model is not an optimisation, it is a
prerequisite. One config block points the engine at any OpenAI-compatible endpoint, local or remote.

## 6. Give the brain a purpose, in advise mode

The purpose text is both an admission filter and a prompt: the agent receives it with the context and learns what
belongs here. Keep it in `advise` while you watch what it scores low. Turning on `enforce` before you have read
those scores is how knowledge disappears without a trace.

## 7. What the agent should do at the end of a turn

The loop that keeps a brain alive, as one procedure:

1. `CONTEXT FOR` the question at depth 1; widen only if needed.
2. Answer from the slice, citing ids.
3. If a decision was made, `REMEMBER` it as kind `decision` and `LINK` it to the question it answers.
4. If it replaces an older rule, `SUPERSEDE` that one. Do not `FORGET`.
5. Leave the open question in the brain as a `question`, so the next turn starts from it.

A brain that is only read goes stale at the half-life of its facts. A brain that is written at the end of every
turn is the only kind that compounds.
