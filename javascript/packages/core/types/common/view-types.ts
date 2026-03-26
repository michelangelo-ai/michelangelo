/**
 * Represents the type of Schema Driven view that is being rendered.
 */
export type View = 'form' | 'detail' | 'list';

/**
 * Union type of all possible view types in the studio.
 * Extends View with additional special view types.
 * @see View for the base view types ('form' | 'detail' | 'list')
 *
 * 'base' is a special view type that is used to represent the base parameters
 * for all views registered to the Schema Driven UI router.
 *
 * 'form-detail' is a special view type that is used to represent the parameters
 * for combined form-detail views.
 *
 * 'unregistered' is a special view type that is used to represent views that are not registered
 * to the Schema Driven UI router but are still part of the studio application.
 */
export type StudioParamsView = View | 'base' | 'form-detail' | 'unregistered';
