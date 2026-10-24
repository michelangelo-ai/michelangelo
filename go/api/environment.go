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
// should call, so the check is not reimplemented locally.
//
//   - restrictedBranches[environment] absent, or an empty/nil slice:
//     always true (unrestricted). The map's keys need not cover every
//     environment name in use; absence means "don't restrict this one".
//     Note that the shipped Helm default restricts "production" (see
//     helm/michelangelo/values.yaml), so a default install is not
//     unrestricted once call sites use this predicate.
//   - Otherwise: true iff branch case-sensitively matches one entry in
//     restrictedBranches[environment]. Matching is exact: no patterns
//     (e.g. "release/*") and no "refs/heads/" prefix handling.
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
