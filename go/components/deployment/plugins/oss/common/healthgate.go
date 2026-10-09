package common

// HealthGateReasonAnnotation records, on the Deployment, why the health check gate last
// judged an in-progress rollout unhealthy. The controller only keeps a generic "Alert fired"
// rollback reason, so this is where operators find the failing cluster or metric.
const HealthGateReasonAnnotation = "deployment.michelangelo.ai/health-gate-reason"
