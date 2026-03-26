export type ErrorEntry = {
  fieldPath: string;
  fieldLabel?: string;

  // Supports Markdown
  errorMessage: string;
  focus?: () => void;
};
