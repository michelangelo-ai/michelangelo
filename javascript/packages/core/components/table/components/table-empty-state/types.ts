export interface EmptyState {
  title: string;
  content?: string;
  icon?: React.ReactNode;
}

export interface TableEmptyStateProps {
  emptyState: EmptyState;
}
