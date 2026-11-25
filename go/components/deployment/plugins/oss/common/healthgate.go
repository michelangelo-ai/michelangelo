package common

// HealthGateReasonAnnotation records, on the Deployment, why the health check gate last
// judged an in-progress rollout unhealthy. The controller only keeps a generic "Alert fired"
// rollback reason, so this is where operators find the failing cluster or metric.
const HealthGateReasonAnnotation = "deployment.michelangelo.ai/health-gate-reason"

// MetricGateNoDataAnnotation lists, on the Deployment, the metric gate queries that returned
// no samples or only NaN, as "<model>: <query>, <query>", cut short with "..." when longer
// than 1024 bytes. Such a query cannot breach, so the candidate was not judged on it. The
// value comes from the most recent evaluation during a rollout that answered every query; the
// gate does not run during a rollback. The annotation is removed once every query returns
// samples, or when a query fails and the value names another model. It is for people to read:
// a query name may contain ", ", so do not parse it.
const MetricGateNoDataAnnotation = "deployment.michelangelo.ai/metric-gate-no-data"
