import type { ExtractEntityFromResponse, HasTypeName } from './types';

/**
 * Extracts the main entity from a response
 *
 * @example
 * ```ts
 * type MyResponse = {
 *   $typeName: 'michelangelo.api.v2.GetProjectResponse';
 *   project: Project;
 * }
 *
 * expect(extractEntityFromResponse(myResponse)).toEqual(myResponse.project)    ;
 *
 * // If the response is not a valid response, it will throw an error
 * type MyResponse = {
 *   $typeName: 'some.other.api.v1.SomeOtherResponse';
 *   someOtherEntity: SomeOtherEntity;
 * }
 *
 * expect(extractEntityFromResponse(myResponse)).toThrowError(
 *   'Entity name someOtherEntity not found in response'
 * );
 * ```
 */
export function extractEntityFromResponse<T extends HasTypeName>(
  response: T
): ExtractEntityFromResponse<T> {
  const typeName = response.$typeName;
  const entityName = typeName
    .replace(/^michelangelo\.api\.v2\.(Get|Create|Update)/, '')
    .replace(/Response$/, '')
    .toLowerCase();

  if (entityName in response) {
    return response[entityName] as ExtractEntityFromResponse<T>;
  }

  throw new Error(`Entity name ${entityName} not found in response`);
}
