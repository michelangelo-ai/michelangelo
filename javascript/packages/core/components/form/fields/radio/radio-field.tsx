import React from 'react';

import { CardRadioField } from './card-radio-field';
import { InlineRadioField } from './inline-radio-field';

import type { RadioFieldProps } from './types';

export const RadioField: React.FC<RadioFieldProps> = (props) => {
  const shouldUseCardRadio = props.options.some((option) => !!option.description);

  return shouldUseCardRadio ? <CardRadioField {...props} /> : <InlineRadioField {...props} />;
};
