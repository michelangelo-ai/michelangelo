package k8sengine

import (
	corev1 "k8s.io/api/core/v1"
	k8sptr "k8s.io/utils/ptr"
)

// WithCredentialsSecret returns envFrom with the object-storage credentials
// Secret (LogPersistenceConfig.CredentialsSecret, holding AWS_ACCESS_KEY_ID and
// AWS_SECRET_ACCESS_KEY) attached as an optional source ahead of every other
// source. Task containers read their settings from the michelangelo-config
// ConfigMap and their credentials from this Secret, the same one the collector
// sidecar reads.
//
// Ahead means index 0. When a key appears in more than one envFrom source,
// Kubernetes takes the value from the last source, so every source the caller
// listed keeps precedence over the Secret: a ConfigMap that still carries
// credentials wins, and hand-built clusters see no change. Optional means a pod
// on a cluster without the Secret still starts; it just gets no variables from
// it.
//
// envFrom comes back unchanged when secret is empty (none configured) or when
// it already loads that Secret without a prefix, which yields the same
// variables; a caller who listed the Secret keeps their own position and
// optional flag. A prefixed entry does not count, since its variables have
// other names. Otherwise the result is a new slice, so the caller's backing
// array is never written to.
func WithCredentialsSecret(envFrom []corev1.EnvFromSource, secret string) []corev1.EnvFromSource {
	if secret == "" || loadsSecret(envFrom, secret) {
		return envFrom
	}
	attached := make([]corev1.EnvFromSource, 0, len(envFrom)+1)
	attached = append(attached, corev1.EnvFromSource{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: secret},
			Optional:             k8sptr.To(true),
		},
	})
	return append(attached, envFrom...)
}

// loadsSecret reports whether envFrom already loads secret without a prefix.
func loadsSecret(envFrom []corev1.EnvFromSource, secret string) bool {
	for _, source := range envFrom {
		if source.Prefix == "" && source.SecretRef != nil && source.SecretRef.Name == secret {
			return true
		}
	}
	return false
}

// attachCredentialsSecret applies WithCredentialsSecret to every container in
// podTemplate. Init containers are left alone: the runtime contract covers the
// Ray containers themselves.
func attachCredentialsSecret(podTemplate *corev1.PodTemplateSpec, secret string) {
	if secret == "" || len(podTemplate.Spec.Containers) == 0 {
		return
	}
	// Build a new slice rather than editing in place: podTemplate is a shallow
	// copy of the caller's v2pb pod template, so its slice header still points at
	// the caller's backing array.
	containers := make([]corev1.Container, len(podTemplate.Spec.Containers))
	copy(containers, podTemplate.Spec.Containers)
	for i := range containers {
		containers[i].EnvFrom = WithCredentialsSecret(containers[i].EnvFrom, secret)
	}
	podTemplate.Spec.Containers = containers
}
