import { useLocation } from 'react-router-dom-v5-compat';

/**
 * Parses URL query string parameters and returns them as a typed object.
 *
 * All query parameter values are returned as strings. For URLs with duplicate
 * parameter names, only the last value is retained (standard URLSearchParams behavior).
 *
 * @template T - The expected shape of query parameters (all values must be strings)
 *
 * @returns Partial object containing all query parameters from the current URL.
 *   Returns empty object {} if no query parameters are present.
 *
 * @example
 * ```typescript
 * // URL: /pipelines?name=training&version=v2
 * type QueryParams = {
 *   name: string;
 *   version: string;
 *   optional?: string;
 * };
 *
 * const params = useURLQueryString<QueryParams>();
 * // params = { name: 'training', version: 'v2' }
 *
 * // URL: /pipelines (no query params)
 * const params = useURLQueryString<QueryParams>();
 * // params = {}
 *
 * // Access with safety checks
 * const name = params.name ?? 'default-name';
 * ```
 */
export function useURLQueryString<T extends Record<string, string>>(): Partial<T> {
  const location = useLocation();
  const { search = '' } = location ?? {};
  return Object.fromEntries(new URLSearchParams(search)) as Partial<T>;
}
