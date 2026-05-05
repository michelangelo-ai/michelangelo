export type MiddlewareOperation = {
  source?: string;
  destination: string;
  default?: unknown;
  transformation?: 'unset' | ((source: unknown) => unknown);
  subTypes?: string[];
};

export type MiddlewareSchema = {
  operations?: MiddlewareOperation[];
  scaffold?: string;
  scaffoldBySubType?: Record<string, string>;
  subTypePath?: string;
};

export type MiddlewareOptions = {
  sourceFromObject?: object;
};
