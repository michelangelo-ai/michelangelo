# Michelangelo Sandbox UI Routes

Route pattern: `http://localhost:5173/{project}/{phase}/{entity}`

Use `ma-dev-test` as the project for all local verification.

## Phase keywords

| Phase | Matches these keywords in the feature description |
|-------|--------------------------------------------------|
| `train` | train, training, model training, learning, fitting, pipeline run |
| `deploy` | deploy, serving, inference, production, online, endpoint |

## Entity keywords

| Entity | Matches these keywords |
|--------|----------------------|
| `pipeline` | pipeline, workflow, dag, steps |
| `run` | run, execution, job, attempt, history |
| `trigger` | trigger, schedule, cron, automated, recurring |
| `model` | model, artifact, version, checkpoint, weights |
| `target` | target, serving target |
| `deployment` | deployment, rollout, canary, release |

## Route table

| Phase | Entity | URL |
|-------|--------|-----|
| train | pipeline | `http://localhost:5173/ma-dev-test/train/pipeline` |
| train | run | `http://localhost:5173/ma-dev-test/train/run` |
| train | trigger | `http://localhost:5173/ma-dev-test/train/trigger` |
| train | model | `http://localhost:5173/ma-dev-test/train/model` |
| deploy | target | `http://localhost:5173/ma-dev-test/deploy/target` |
| deploy | deployment | `http://localhost:5173/ma-dev-test/deploy/deployment` |

## Resolution algorithm

Apply in order:

1. **Resolve phase** — scan the feature description for phase keywords (table above). Record all matching phases.
2. **Resolve entity** — scan the feature description for entity keywords (table above). Record all matching entities.
3. **Look up (phase, entity)** — find the row in the route table that matches both resolved signals.

**IF** exactly one (phase, entity) pair matches → use that URL.

**IF** the entity exists in multiple phases (collision) AND no phase keyword was found:
- Default to `train` phase.
- Note in the Step 5 report: *"Assumed train/{entity}; if you meant deploy/{entity}, re-invoke with the phase specified."*

**IF** no entity keyword matches:
- Start at `http://localhost:5173/ma-dev-test/train/pipeline`.
- Note in the report that no entity could be inferred.

**IF** a (phase, entity) combination appears in the description but has no row in the route table:
- Stop. Report: *"No route found for {phase}/{entity}. Available routes: [list table]."* Do not guess.
