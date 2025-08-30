import React from 'react';
import { Input, SIZE } from 'baseui/input';

import { Icon } from '#core/components/icon/icon';

import type { TableSearchInputProps } from './types';

export function TableSearchInput({ value, onChange }: TableSearchInputProps) {
  const [localValue, setLocalValue] = React.useState(value);

  // Sync local state with external value (for navigation between tables)
  React.useEffect(() => {
    setLocalValue((current) => (current === value ? current : value));
  }, [value]);

  const debouncedOnChange = React.useMemo(() => {
    let timeoutId: NodeJS.Timeout;
    return (newValue: string) => {
      clearTimeout(timeoutId);
      timeoutId = setTimeout(() => onChange(newValue), 500);
    };
  }, [onChange]);

  const persistNewSearchValue = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    setLocalValue(newValue);
    debouncedOnChange(newValue);
  };

  return (
    <Input
      clearable
      type="search"
      value={localValue}
      onChange={persistNewSearchValue}
      size={SIZE.compact}
      overrides={{
        Root: { style: { width: '250px' } },
        ClearIcon: { props: { size: 20 } },
      }}
      placeholder="Search..."
      startEnhancer={<Icon name="search" />}
    />
  );
}
