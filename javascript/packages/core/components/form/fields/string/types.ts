import type { BaseFieldProps } from '../types';

export type SingleStringFieldProps = BaseFieldProps<string> & { multi?: false };
export type MultiStringFieldProps = BaseFieldProps<string[]> & { multi: true };

export type StringFieldProps = SingleStringFieldProps | MultiStringFieldProps;
