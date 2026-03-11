import type { FormRowProps } from '#core/components/form/layout/form-row/types';
import type { ArrayLayoutProps } from '#core/components/form/layout/types';

export interface ArrayFormRowProps extends Omit<FormRowProps, 'children'>, ArrayLayoutProps {}
