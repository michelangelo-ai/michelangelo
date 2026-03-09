import React, { useMemo } from 'react';

import { CardRadioField } from './card-radio-field';
import { InlineRadioField } from './inline-radio-field';

import type { RadioFieldProps } from './types';

export const RadioField: React.FC<RadioFieldProps> = (props) => {
  const shouldUseCardRadio = useMemo(
    () => props.options.some((option) => option.description != null),
    [props.options]
  );

  return shouldUseCardRadio ? <CardRadioField {...props} /> : <InlineRadioField {...props} />;
};
