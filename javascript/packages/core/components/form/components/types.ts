export interface FormControlProps {
  label?: string;
  required?: boolean;
  description?: string;
  caption?: string;
  error?: string;
  children: React.ReactNode;
}
