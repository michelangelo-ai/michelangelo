// Package declarative decodes, validates, and plans a DeclarativeWorkflow.
//
// A PipelineManifest carries a DeclarativeWorkflow in an Any. Use the
// functions in this order:
//
//  1. LoadReleases or ParseRelease reads the interpreter releases. A Release
//     maps each task type (the task_function of a task) to the schemas of its
//     config and job_specs, and lists its capabilities.
//  2. Decode unpacks the manifest. It rejects unknown fields at any level.
//  3. Validate checks the workflow on its own: names, limits, the depends_on
//     graph, parameters, and references.
//  4. ValidateAgainst runs Validate, then checks each task against the
//     schemas of one Release.
//  5. Plan runs ValidateAgainst and returns an ExplainPlan, a dry-run preview
//     that groups the tasks into levels.
//
// A string value in config or job_specs can hold references:
// {{ params.NAME }} reads a parameter, and {{ tasks.NAME.output[.FIELD...] }}
// reads the output of another task. A task can read the output of a task
// only when that task is a dependency and has succeeded whenever the reader
// runs.
//
// Every problem in the content of a workflow is reported as an *Error.
package declarative
