import { MultiStringField } from './components/multi-string-field';
import { SingleStringField } from './components/single-string-field';

import type { StringFieldProps } from './types';

export function StringField(props: StringFieldProps) {
  return props.multi ? <MultiStringField {...props} /> : <SingleStringField {...props} />;
}
