export interface PageHeaderProps {
  /**
   * Icon name from the icon provider system.
   * When provided, creates a 2-column grid layout with icon in the left column.
   */
  icon?: string;

  /** Main heading text displayed in the header */
  label: string;

  /**
   * Optional description text displayed below the label.
   * Supports inline documentation link when combined with docUrl.
   */
  description?: string;

  /**
   * Optional URL to external documentation.
   * When provided with description, renders a "Learn more" button with arrow icon.
   */
  docUrl?: string;
}
