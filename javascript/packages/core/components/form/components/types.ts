export interface FormControlProps {
  label?: string;
  required?: boolean;
  description?: string;
  labelAddon?: React.ReactNode;
  /** Rendered as Markdown. */
  caption?: string;
  error?: string;
  counter?: {
    length: number;
    maxLength: number;
  };
  children: React.ReactNode;
}
