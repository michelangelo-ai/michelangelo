package common

// HealthGateReasonAnnotation records, on the Deployment, why the health check gate last
// judged an in-progress rollout unhealthy. The controller only keeps a generic "Alert fired"
// rollback reason, so this is where operators find the failing cluster or metric.
const HealthGateReasonAnnotation = "deployment.michelangelo.ai/health-gate-reason"

// MetricGateNoDataAnnotation lists, on the Deployment, the metric gate queries that returned
// no samples or only NaN, as "<model>: <query>, <query>". Such a query cannot breach, so the
// candidate was not judged on it. The value comes from the last evaluation during a rollout
// in which every query was answered, and the annotation is removed once every query returns
// samples.
const MetricGateNoDataAnnotation = "deployment.michelangelo.ai/metric-gate-no-data"
