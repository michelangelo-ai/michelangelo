export interface FormProps {
  onSubmit: (values: Record<string, unknown>) => void | object | Promise<object>;
  initialValues?: Record<string, unknown>;
  /** Form ID for external submit button integration in modals */
  id?: string;
  children: React.ReactNode;
}

export interface FormState {
  submitting: boolean;
  hasErrors: boolean;
  dirty: boolean;
  valid: boolean;
}

export interface FieldState {
  error?: string;
  touched: boolean;
}

export interface FieldInput<T = unknown> {
  value: T;
  name: string;
  onChange: (value: T) => void;
  onBlur: () => void;
}
