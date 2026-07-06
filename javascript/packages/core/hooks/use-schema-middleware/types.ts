import type { StudioParamsBase } from '#core/hooks/routing/use-studio-params/types';

export type MiddlewareOperation = {
  source?: string;
  destination: string;
  /**
   * Value to set when `source` is absent or resolves to nil.
   * Can be a function `({ studio }) => value` to derive the default from routing context.
   */
  default?: ((context: { studio: StudioParamsBase }) => unknown) | string | number | boolean | null;
  /**
   * Function applied to the source value before writing to `destination`.
   * Use `'unset'` to delete the destination path from the record entirely.
   */
  transformation?: 'unset' | ((source: unknown) => unknown);
  /**
   * When set, this operation only runs if the record's subType (at `subTypePath`) is one of these values.
   * Requires `subTypePath` on the schema.
   */
  subTypes?: string[];
};

export type MiddlewareSchema = {
  operations?: MiddlewareOperation[];
  /**
   * YAML string merged into the record as defaults before operations run.
   * Existing data values always win over scaffold values.
   */
  scaffold?: string;
  /**
   * Per-subType YAML scaffolds, keyed by the value at `subTypePath`.
   * Requires `subTypePath`.
   */
  scaffoldBySubType?: Record<string, string>;
  /** Dot-path into the record used to read the subType for `subTypes` filtering and `scaffoldBySubType`. */
  subTypePath?: string;
};

export type MiddlewareOptions = {
  /**
   * When provided, operation `source` paths are read from this object instead of the data record.
   * Results are still written to the data record.
   */
  sourceFromObject?: object;
};
