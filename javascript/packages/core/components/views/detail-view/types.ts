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
}
