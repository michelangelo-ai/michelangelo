export interface RevisionSelectorProps {
  /** Service of the revisioned entity, e.g. `pipeline` */
  service: string;
  /** Name of the entity whose revisions are listed */
  entityId: string;
  /** Revision id currently shown, from the `?revisionId=` query param */
  selectedRevisionId?: string;
  /** `metadata.name` of the Revision the entity's `status.latestRevision` points at */
  latestRevisionName?: string;
  onSelect: (revisionId: string) => void;
}

/** The slice of a Revision CR the selector renders. */
export interface RevisionOption {
  metadata: { name: string; creationTimestamp?: { seconds?: string | number } };
  spec: { revisionId: string; gitCommit?: { branch?: string } };
}
