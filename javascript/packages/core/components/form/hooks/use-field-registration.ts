import { useEffect } from 'react';

import { useFormContext } from '#core/components/form/form-context';

export function useFieldRegistration(name: string, label: string | undefined): void {
  const { fieldRegistry } = useFormContext();

  useEffect(() => {
    if (label) fieldRegistry.set(name, { label });
    return () => {
      fieldRegistry.delete(name);
    };
  }, [name, label, fieldRegistry]);
}
