import { isURL } from 'validator';

export const capitalizeFirstLetter = (str: string): string =>
  str.charAt(0).toUpperCase() + str.slice(1);

export const isAbsoluteURL = (value: string) => isURL(encodeURI(value), { require_protocol: true });
