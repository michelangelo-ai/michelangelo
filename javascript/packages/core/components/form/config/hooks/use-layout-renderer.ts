import { useFormProvider } from '#core/providers/form-provider/use-form-provider';
import { LAYOUT_RENDERERS } from '../constants';

import type { LayoutRenderer } from '../types';

export function useLayoutRenderer(type: string): LayoutRenderer | undefined {
  const formContext = useFormProvider();
  return formContext?.layouts[type] ?? LAYOUT_RENDERERS[type];
}
