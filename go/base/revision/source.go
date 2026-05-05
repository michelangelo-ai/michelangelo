package revision

// Well-known values for RevisionSpec.source. Third-party controllers SHOULD
// namespace their own values (e.g. "acme.io/NightlyRetrain") to avoid
// colliding with future platform-defined sources.
const (
	SourceGit            = "Git"
	SourceResourceUpdate = "ResourceUpdate"
	SourceUnknown        = "Unknown"
)
