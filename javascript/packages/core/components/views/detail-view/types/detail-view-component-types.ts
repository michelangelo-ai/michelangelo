import type { ActionConfig, Data } from '#core/components/actions/types';

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

  actions?: ActionConfig[];
  record?: Data;
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
