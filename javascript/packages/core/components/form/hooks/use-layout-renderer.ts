import { LAYOUT_RENDERERS } from '#core/components/form/constants';
import { useFormProvider } from '#core/providers/form-provider/use-form-provider';

import type { LayoutRenderer } from '#core/components/form/types/form-types';

export function useLayoutRenderer(type: string): LayoutRenderer | undefined {
  const formContext = useFormProvider();
  return formContext?.layouts[type] ?? LAYOUT_RENDERERS[type];
}
