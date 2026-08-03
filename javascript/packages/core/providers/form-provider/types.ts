import type { FieldRenderer, LayoutRenderer } from '#core/components/form/types/form-types';

export type FormContextType = {
  renderers: Record<string, FieldRenderer>;
  layouts: Record<string, LayoutRenderer>;
};
