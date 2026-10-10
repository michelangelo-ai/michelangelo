package api

// IsBranchAllowedForEnvironment reports whether a commit on branch may be
// labeled with the given EnvironmentLabel value environment, given the
// operator's configured restrictedBranches map
// (apihandler.Config.RestrictedBranches). restrictedBranches is keyed by
// environment name (e.g. "production") rather than hardcoding any one
// environment value: EnvironmentLabel is itself a fully operator-defined
// free string with no enum anywhere in this package (see EnvironmentLabel's
// own doc comment), so hardcoding "production" as the one restrictable
// value here would be inconsistent with that — and would silently give
// zero protection to an operator whose production-equivalent environment
// is named something else (e.g. "prod", "release").
//
// This is the single predicate every EnvironmentLabel-defaulting call site
// (pipelinerun/apihook, triggerrun/apihook, and any future one) must call —
// do not reimplement this check locally. Centralizing it here is a direct
// response to the (genericized, non-internal-specific) finding in
// research.md that repo-local, ad-hoc reimplementations of "is this a
// production branch" tend to drift out of sync with each other over time.
//
//   - restrictedBranches[environment] absent, or an empty/nil slice:
//     always true (unrestricted). An operator who has not opted a given
//     environment name into this map gets today's unrestricted behavior —
//     this is what makes the feature fully backward compatible (see §5)
//     and means the map's keys never need to be exhaustive over every
//     environment name in use; absence is the "don't restrict this one"
//     signal, not an error.
//   - Otherwise: true iff branch case-sensitively matches one entry in
//     restrictedBranches[environment].
func IsBranchAllowedForEnvironment(environment, branch string, restrictedBranches map[string][]string) bool {
	allowed, ok := restrictedBranches[environment]
	if !ok || len(allowed) == 0 {
		return true
	}
	for _, b := range allowed {
		if b == branch {
			return true
		}
	}
	return false
}
