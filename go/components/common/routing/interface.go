//go:generate mamockgen Manager

// Package routing defines the CRD-agnostic interface for managing routing objects.
package routing

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// PathMatchType and RewriteType are semantic routing constants.
type (
	PathMatchType string
	RewriteType   string
)

const (
	PathMatchExact  PathMatchType = "Exact"    // matches the full path
	PathMatchPrefix PathMatchType = "Prefix"   // matches any path with this prefix
	RewriteFullPath RewriteType   = "FullPath" // replaces the entire path
	RewritePrefix   RewriteType   = "Prefix"   // replaces only the matched prefix
)

// OwnerRef is a Kubernetes owner reference.
type OwnerRef struct {
	APIVersion string
	Kind       string
	Name       string
	UID        types.UID
}

// Rule is a routing rule: match a path, optionally rewrite it, forward to a
// named backend Service. BackendPort defaults to 80 when zero.
type Rule struct {
	MatchPath   string
	MatchType   PathMatchType // defaults to Exact when zero
	RewritePath string        // empty = no rewrite
	RewriteType RewriteType   // ignored when RewritePath is empty
	BackendName string
	BackendPort int32
}

// RouteConfig holds the configuration for a routing object to be created.
type RouteConfig struct {
	GatewayName      string
	GatewayNamespace string
	OwnerRef         *OwnerRef // nil = no owner reference
	Rules            []Rule    // initial rules (may be empty)
}

// Manager manages routing objects (Routes) and the per-path entries within
// them (Rules). Implementations should encapsulate all CRD-specific construction;
// callers work exclusively with RouteConfig and Rule.
//
// A Route is the top-level k8s object that attaches to a Gateway and carries
// the rules that govern incoming traffic. It is addressable by (name,
// namespace). Create provisions it, Delete tears it down, Exists reports
// presence. Concrete implementations: Gateway API HTTPRoute, Istio
// VirtualService.
//
// A Rule is one entry inside a Route's rule list, identified by MatchPath
// (at most one rule per MatchPath per Route). Each rule configures how the
// Route behaves for a given path. AddRules upserts by MatchPath, RemoveRules
// deletes by it. A Route with multiple rules dispatches different paths to
// different backends, all under the same Gateway attachment.
type Manager interface {
	// Create creates the route if it does not already exist.
	Create(ctx context.Context, client dynamic.Interface, name, namespace string, config RouteConfig) error
	// Exists reports whether the route has been created.
	Exists(ctx context.Context, client dynamic.Interface, name, namespace string) (bool, error)
	// Delete removes the route. Tolerates not-found.
	Delete(ctx context.Context, client dynamic.Interface, name, namespace string) error
	// AddRules upserts each rule by MatchPath. Returns an error if the route does not exist.
	AddRules(ctx context.Context, client dynamic.Interface, name, namespace string, rules ...Rule) error
	// RemoveRules removes the rules identified by matchPaths. No-op when absent.
	RemoveRules(ctx context.Context, client dynamic.Interface, name, namespace string, matchPaths ...string) error
	// RuleExists reports whether a rule matching the given criteria exists.
	// When rule.RewritePath is non-empty, both path and rewrite must match.
	RuleExists(ctx context.Context, client dynamic.Interface, name, namespace string, rule Rule) (bool, error)
}
