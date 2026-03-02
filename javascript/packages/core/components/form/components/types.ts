export interface FormControlProps {
  label?: string;
  required?: boolean;
  description?: string;
  caption?: string;
  error?: string;
  counter?: {
    length: number;
    maxLength: number;
  };
  children: React.ReactNode;
}
