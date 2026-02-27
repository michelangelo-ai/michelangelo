export interface FormControlProps {
  label?: string;
  required?: boolean;
  description?: string;
  caption?: string;
  error?: string;
  labelEndEnhancer?: React.ReactNode;
  children: React.ReactNode;
}
