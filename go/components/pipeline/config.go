package pipeline

// Config holds runtime configuration for the pipeline controller.
type Config struct {
	// RevisioningEnabled controls whether the controller snapshots a Revision CR on
	// each successful reconcile. Defaults to false — enable only when a
	// revision.Manager is wired in and revision storage is available.
	RevisioningEnabled bool `yaml:"revisioningEnabled"`
}
