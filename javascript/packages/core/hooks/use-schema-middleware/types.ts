export type MiddlewareOperation = {
  source?: string;
  destination: string;
  default?: unknown;
  transformation?: 'unset' | ((source: unknown) => unknown);
};

export type MiddlewareSchema = {
  operations?: MiddlewareOperation[];
};

export type MiddlewareOptions = {
  sourceFromObject?: object;
};
