/**
 * @description
 * Type for objects that have a $typeName property. $typeName is added by the `@bufbuild/protobuf` library.
 * This type is used to identify the associated protobuf message type.
 *
 * @example
 * ```ts
 * type GetProjectResponse = {
 *   $typeName: 'michelangelo.api.v2.GetProjectResponse';
 *   project: Project;
 * };
 * ```
 */
export type HasTypeName = {
  $typeName: string;
};

/**
 * @description
 * Singular endpoints are endpoints that return a single entity.
 *
 * @example
 * ```ts
 * type GetProjectResponse = {
 *   $typeName: 'michelangelo.api.v2.GetProjectResponse';
 *   project: Project;
 * };
 */
type SingularEndpoint = 'Get' | 'Create' | 'Update';

/**
 * @description
 * Delete endpoint is the endpoint that deletes an entity.
 */
type DeleteEndpoint = 'Delete';

/**
 * @description
 * List endpoint is the endpoint that returns a list of entities.
 */
type ListEndpoint = 'List';

/**
 * @description
 * Type for the endpoint part of the $typeName property.
 *
 * @example
 * ```ts
 * // Endpoint can be used in a type guard to check if a response typeName
 * // belongs to a known endpoint
 * type IsProjectResponse<T extends string> =
 *   T extends `michelangelo.api.v2.${Endpoint}ProjectResponse` ? true : false;
 *
 * type IsProjectResponseResult = IsProjectResponse<'michelangelo.api.v2.GetProjectResponse'>;
 * // => true -- Get is not a known endpoint
 *
 * type IsProjectResponseResult = IsProjectResponse<'michelangelo.api.v2.RetryProjectResponse'>;
 * // => false -- Retry is not a known endpoint
 * ```
 */
type Endpoint = SingularEndpoint | ListEndpoint | DeleteEndpoint;

/**
 * @description
 * Extracts the lowercased entity name from an absolute path to the response protobuf message.
 * The resulting entity name should be used as the key to access the entity from the response object.
 *
 * @remarks
 * This assumes that the response protobuf message is in the `michelangelo.api.v2` namespace.
 *
 * @example
 * ```ts
 * type GetProjectResponse = {
 *   $typeName: 'michelangelo.api.v2.GetProjectResponse';
 *   project: Project;
 * };
 *
 * ExtractEntityName<'michelangelo.api.v2.GetProjectResponse'>;
 * // => 'project'
 *
 * ExtractEntityName<'some.other.protobuf.GetProjectResponse'>;
 * // => never
 * ```
 */
type ExtractEntityName<T extends string> =
  T extends `michelangelo.api.v2.${Endpoint}${infer Entity}Response` ? Lowercase<Entity> : never;

/**
 * @description
 * Extracts the inner entity from a protobuf response message.
 *
 * @example
 * ```ts
 * type GetProjectResponse = {
 *   $typeName: 'michelangelo.api.v2.GetProjectResponse';
 *   project: Project;
 * };
 *
 * ExtractEntityFromResponse<GetProjectResponse>;
 * // => Project
 *
 * type ListProjectsResponse = {
 *   $typeName: 'michelangelo.api.v2.ListProjectsResponse';
 *   projects: Project[];
 * };
 *
 * ExtractEntityFromResponse<ListProjectsResponse>;
 * // => Project[]
 *
 * type SomeOtherResponse  = {
 *   $typeName: 'some.other.protobuf.SomeOtherResponse';
 *   someField: string;
 * };
 *
 * ExtractEntityFromResponse<SomeOtherResponse>;
 * // => SomeOtherResponse
 * ```
 */
export type ExtractEntityFromResponse<T> = T extends HasTypeName
  ? T extends Partial<Record<ExtractEntityName<T['$typeName']>, infer E>>
    ? E
    : T
  : T;
