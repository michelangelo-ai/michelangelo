export interface RevisionSelectorProps {
  /** Service of the revisioned entity, e.g. `pipeline` */
  service: string;
  /** Name of the entity whose revisions are listed */
  entityId: string;
  /** Revision id currently shown, from the `?revisionId=` query param */
  selectedRevisionId?: string;
  onSelect: (revisionId: string) => void;
}

/** The slice of a Revision CR the selector renders. */
export interface RevisionOption {
  metadata: { name: string; creationTimestamp?: { seconds?: string | number } };
  spec: { revisionId: string; gitCommit?: { branch?: string } };
}
