import type { ActionConfig } from '#core/components/actions/types';

export interface DetailViewProps extends DetailHeaderBaseProps {
  /**
   * Content displayed at the bottom of the header container
   */
  headerContent?: React.ReactNode;

  children: React.ReactNode;
}

export interface DetailHeaderBaseProps {
  /**
   * Small text displayed above the main title in the header
   */
  subtitle?: string;

  /**
   * Main heading displayed next to the back button
   */
  title?: string;
  onGoBack?: () => void;

  /**
   * Configuration for set of action buttons that render within the header. These actions
   * operate on the currently viewed entity record. For example, a "Delete" action would delete
   * the currently viewed entity.
   */
  actions?: ActionConfig[];

  /** The data for the currently viewed entity. */
  record?: Record<string, unknown>;

  /** Loading state for the currently viewed entity. Indicates that record data may be incomplete. */
  loading?: boolean;
}

export interface DetailViewTab {
  id: string;
  label: string;
  content: React.ReactNode;
}

export interface DetailViewPagesProps {
  tabs: DetailViewTab[];
  activeTabId?: string;
  onTabSelect?: (tabId: string) => void;
}
