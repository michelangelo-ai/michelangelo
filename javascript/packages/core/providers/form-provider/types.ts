import type { FieldRenderer, LayoutRenderer } from '#core/components/form/types';

export type FormContextType = {
  renderers: Record<string, FieldRenderer>;
  validators: Record<string, (...args: never[]) => (value: unknown) => string | undefined>;
  layouts: Record<string, LayoutRenderer>;
};
