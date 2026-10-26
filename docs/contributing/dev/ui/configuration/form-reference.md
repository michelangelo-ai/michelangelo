---
last_verified: 2026-10-26
---

# Form Reference

Forms in Michelangelo AI UI are **React components**, not configuration objects. You build a form by putting field components inside `Form` (or `FormDialog`, for a form in a modal) and arranging them with layout components. Every form in the app works this way, including the "Create deployment", "Create target" and "Run pipeline" forms. Form state is managed by [React Final Form](https://final-form.org/react).

This page is for contributors building forms for actions and custom detail pages in Michelangelo AI Studio. It assumes you've read the [Action Configuration Reference](./action-configuration-reference.md).

Forms usually appear in two places:

- A **custom action modal** or a **`createAction`** component (see [Action Configuration Reference](./action-configuration-reference.md))
- A **custom detail page** that shows record values as read-only fields

**Imports:**
- `Form`, `FormDialog`, the field and layout components, and the hooks below are all exported from `@michelangelo-ai/core`.
- The validators (`required`, `combineValidators`, …) are **not** exported from the package.
- Inside `packages/core`, import everything from `#core/components/form/...`, as the config files do. The examples on this page follow that style.

A minimal form:

```tsx
import { Form, StringField, SubmitButton } from '@michelangelo-ai/core';

<Form onSubmit={(values) => console.log(values)}>
  <StringField name="metadata.name" label="Name" required />
  <SubmitButton>Save</SubmitButton>
</Form>
```

## Form

| Property        | Type                                                          | Description                                                                                       | Default/Required |
| --------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ---------------- |
| `onSubmit`      | `(values) => void \| object \| Promise<object>`               | Called with the form values on submit. Return or resolve `{ [FORM_ERROR]: … }` (from `final-form`) to report a form-level error | ✅ Required |
| `initialValues` | `DeepPartial<FieldValues>`                                    | Starting values, keyed by field `name`                                                             | `undefined` |
| `id`            | `string`                                                      | `id` of the `<form>` element, so a submit button outside it can target it with `form={id}`        | `undefined` |
| `children`      | `React.ReactNode`                                             | Fields and layout components                                                                       | ✅ Required |
| `focusOnError`  | `boolean`                                                     | Focus the first field with a validation error when a submit fails                                  | `true` |
| `render`        | `(formElement: React.ReactNode) => React.ReactNode`           | Wraps the `<form>` element, so content outside it (e.g. a dialog footer) can still read form state with `useFormState` | `undefined` |
| `footer`        | `{ left?: React.ReactNode; right?: React.ReactNode } \| React.ReactNode` | A sticky footer fixed to the bottom of the viewport. With `{ left, right }`, `right` is usually the submit button | `undefined` |

## FormDialog

A `Form` inside a modal `Dialog`, with a submit button, a Cancel button and a `FormErrorBanner` already wired up. Use it for action modals and `createAction` components.

| Property        | Type                          | Description                                                  | Default/Required |
| --------------- | ----------------------------- | ------------------------------------------------------------ | ---------------- |
| `isOpen`        | `boolean`                     | Whether the dialog is shown                                  | ✅ Required |
| `onDismiss`     | `() => void`                  | Called on Cancel, on close, and **after a successful submit** | ✅ Required |
| `heading`       | `string`                      | Dialog title                                                 | ✅ Required |
| `onSubmit`      | Same as `Form`                | Called with the form values. Resolving, even with `{ [FORM_ERROR]: … }`, closes the dialog; to keep it open and show an error, throw | ✅ Required |
| `initialValues` | Same as `Form`                | Starting values                                              | `undefined` |
| `children`      | `React.ReactNode`             | Fields and layout components                                 | ✅ Required |
| `size`          | `Size` from `baseui/dialog`   | Dialog size                                                  | `SIZE.large` |
| `submitLabel`   | `string`                      | Submit button label                                          | `'Submit'` |

If `onSubmit` resolves, the dialog calls `onDismiss` and closes. If it throws, the dialog stays open and shows the error in the `FormErrorBanner`. Unlike `Form`, a `FormDialog` closes on any resolved `onSubmit`, even one that returns `{ [FORM_ERROR]: … }`, so throw to report an error. You don't need to catch errors from `mutateAsync` yourself.

## Field Props Shared by Every Field (`BaseFieldProps`)

| Property           | Type                          | Description |
| ------------------ | ----------------------------- | ----------- |
| `name`             | `string`                      | **Required.** The key in the form values. Supports final-form dot notation for nested values, e.g. `'spec.pipeline.name'` |
| `label`            | `string`                      | Label shown above the field |
| `defaultValue`     | `T`                           | Value when the field is created. A configured value (e.g. from `initialValues`) takes precedence |
| `initialValue`     | `T`                           | Value on page load. Takes precedence over `defaultValue` and configured values; the user can still change it |
| `required`         | `boolean`                     | Shows the required asterisk **and** adds the `required()` validator |
| `readOnly`         | `boolean`                     | Normal styling but immutable. The value **is** submitted |
| `disabled`         | `boolean`                     | Disabled styling and immutable. See the note below about submission |
| `placeholder`      | `string`                      | Example text shown while the field is empty. See the rules below |
| `description`      | `string`                      | Shown when hovering the label. Markdown supported |
| `caption`          | `string`                      | Shown directly below the field. Markdown supported |
| `labelEndEnhancer` | `React.ReactNode`             | Content at the far right of the label row, such as a button |
| `parse`            | `(value: InputValue) => T`    | Transforms the input value before it's stored in form state |
| `format`           | `(value: T) => InputValue`    | Transforms the stored value for display in the input |
| `validate`         | `(value: unknown) => string \| undefined` | Returns an error message, or `undefined` when valid. Runs on every change; its error is shown once the field is touched |

**`required` already validates.** Every field component passes `required` to `useField`, which prepends `required()` to your `validate`. So `required` alone is enough. Adding `validate={required()}`, or `required()` inside `combineValidators`, runs the same check twice. It's harmless, but redundant.

:::note `disabled` vs `readOnly` on submit
The doc comment on `BaseFieldProps.disabled` says disabled fields are filtered out of the submitted values. **`Form` doesn't do that today.** It hands every value in form state to `onSubmit`, disabled fields included. If a disabled field's value mustn't be sent, remove it in your `onSubmit`.
:::

**Placeholder rules** (from the `placeholder` doc comment):
- Prefix with "e.g." and show an example, not an instruction: `"e.g. my-pipeline"`, not `"Enter pipeline name"`.
- Never repeat the field's `label`.
- Never use action verbs ("Enter…", "Type…", "Select…", "Choose…").
- Leave it out when the label is self-explanatory.

## Field Components

Each field takes the shared props above plus its own:

| Component       | Value type                  | Own props |
| --------------- | --------------------------- | --------- |
| `StringField`   | `string`, or `string[]` with `multi` | `multi?: boolean`: a tag input whose value is a string array. `maxLength?: number` (single-value only): limits length and shows a character counter |
| `TextareaField` | `string`                    | `rows?: number`, `maxLength?: number` (with counter) |
| `MarkdownField` | `string`                    | `rows?: number`, `maxLength?: number` (with counter). With `readOnly`, renders the value as Markdown instead of a textarea |
| `NumberField`   | `number \| undefined`       | None. An empty input is stored as `undefined` |
| `BooleanField`  | `boolean`                   | `checkboxLabel?: string` (default `"Enabled"` when checked, `"Disabled"` when not); `toggle?: boolean` renders a switch instead of a checkbox (default `false`) |
| `SelectField`   | `V`, or `V[]` with `multi`  | `options: SelectOption[]` (`{ id, label, disabled? }`) (required); `multi?`; `clearable?` (default `true`); `searchable?` (default `true`); `creatable?` (default `false`); `isLoading?` (default `false`); `visibleOptionLimit?: number`; `getOptionContent?: (option) => ReactNode` for richer dropdown rows |
| `RadioField`    | `string \| boolean`         | `options: RadioOption[]` (`{ value, label, description?, disabled? }`) (required); `align?` (inline layout only). If **any** option has a `description`, every option renders as a card; otherwise they render as inline radios |
| `CheckboxField` | `string[]`                  | `options: CheckboxOption[]` (`{ id, label, description? }`) (required) |
| `DateField`     | `string`                    | `dateFormat?: DateFormat` (`DateFormat.ISO_DATE_STRING`, the default, or `DateFormat.EPOCH_SECONDS` for int64 epoch-second strings); `noFutureDate?: boolean` (default `false`) |
| `UrlField`      | `string`                    | `urlName?: string`. **Display only:** there's no input. It renders the value as a link (labelled `urlName ?? label ?? value`), or the `placeholder` text when the value isn't a navigable URL |

`DateFormat` is exported from `@michelangelo-ai/core` along with the fields.

## Validators

Validators live in `#core/components/form/validation/validators.ts`. Each one returns a `FieldValidator` (`(value: unknown) => string | undefined`), and the optional `errorMessage` argument overrides its default message.

| Validator              | Fails when                                         | Default message |
| ---------------------- | -------------------------------------------------- | --------------- |
| `required(errorMessage?)`       | The value is empty. Booleans and numbers (including `false` and `0`) always pass | `'This field is required.'` |
| `min(minimum, errorMessage?)`         | `Number(value)` is `NaN` or less than `minimum`          | `` `Must be at least ${minimum}.` `` |
| `max(maximum, errorMessage?)`         | `Number(value)` is `NaN` or greater than `maximum`       | `` `Must be at most ${maximum}.` `` |
| `minLength(length, errorMessage?)`   | `String(value).length < length`                         | `` `Must be at least ${length} characters.` `` |
| `maxLength(length, errorMessage?)`   | `String(value).length > length`                         | `` `Must be at most ${length} characters.` `` |
| `regex(pattern, errorMessage?)` | The value doesn't match `pattern` (string or `RegExp`) | `'Invalid format.'` |
| `url(errorMessage?)`            | The value isn't an absolute URL                    | `'Must be a valid URL.'` |

**Every validator except `required` passes empty values.** Empty means `undefined`, `null`, a blank string, an empty array or an empty object. So `min(1)` alone accepts an empty field; add `required` to make the field mandatory.

Compose validators with `combineValidators(...validators)` from `#core/components/form/validation/combine-validators`. It runs them in order and returns the **first** error.

## Layout Components

| Component        | Props | Use |
| ---------------- | ----- | --- |
| `FormGroup`      | `title?`, `description?` (Markdown), `tooltip?` (Markdown), `endEnhancer?`, `overrides?`, `collapsible?`. With `collapsible: true`, also `expanded?` and `onToggle?(expanded)` for controlled state | A titled section |
| `FormRow`        | `name?` (row label), `span?: number[]` (column span per child; equal widths by default) | Fields side by side |
| `FormColumn`     | `children` only | Stacks fields vertically inside a row |
| `FormBanner`     | `title?`, `kind?: 'info' \| 'warning'` (default `'info'`), `dismissible?` (default `false`; once dismissed it stays hidden while mounted), `content?` (strings render as Markdown) | An inline notice |
| `FormStep`       | `name` (required), `description?` (Markdown) | A section with a heading (`name`) and optional description above its fields |
| `ArrayFormRow`   | `FormRow` props, plus the array props below and `addLabel?` (default `"Add more"`) | One row per array item |
| `ArrayFormGroup` | `FormGroup` props except `title` and `endEnhancer`, plus the array props below, `groupLabel?` (titles groups "Address 1", "Address 2", …) and `addLabel?` (default `` `Add ${groupLabel.toLowerCase()}` ``, or `"Add more"`) | One group per array item |

**Array layout props** (`ArrayLayoutProps`, shared by `ArrayFormRow` and `ArrayFormGroup`):

| Property        | Type                                                      | Description |
| --------------- | --------------------------------------------------------- | ----------- |
| `rootFieldPath` | `string`                                                  | **Required.** Dot-notation path to the array value, e.g. `'addresses'` |
| `children`      | `(indexedFieldPath: string, index: number) => ReactNode`  | **Required.** Called once per item. Prefix field names with `indexedFieldPath` (e.g. `"addresses[0]"`) |
| `minItems`      | `number`                                                  | Pre-fills empty items on mount up to this count, and hides the remove control once only `minItems` items remain. Default `0` |
| `readOnly`      | `boolean`                                                 | Hides the add and remove controls. Default `false` |

No built-in form uses the array layouts yet. This sketch follows the types' own doc comments:

```tsx
<ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={1}>
  {(indexedFieldPath) => (
    <FormRow>
      <StringField name={`${indexedFieldPath}.street`} label="Street" required />
      <StringField name={`${indexedFieldPath}.city`} label="City" />
    </FormRow>
  )}
</ArrayFormGroup>
```

## Hooks and Helper Components

| Export          | Signature / props | Use |
| --------------- | ----------------- | --- |
| `useForm()`     | Returns `{ fieldRegistry, change(name, value), submit() }` | Set another field's value from inside the form |
| `useFormState(subscription?)` | Returns `FormState`: `submitting`, `submitError`, `values`, `submitFailed`, `hasValidationErrors`, `errors`, `submitErrors`, `touched`, `modifiedSinceLastSubmit`. Pass e.g. `{ submitting: true }` to subscribe to only some fields | Read form state |
| `useField(name, options?)` | `options`: `validate`, `required`, `defaultValue`, `initialValue`, `label`, `parse`, `format`. Returns `{ input: { value, name, onChange, onBlur, onFocus }, meta: { error, touched } }` | Build a custom field |
| `FormControl`   | `label?`, `required?`, `description?`, `labelEndEnhancer?`, `caption?`, `error?`, `counter?`, `children` | Label, caption and error chrome for a custom field |
| `SubmitButton`  | baseui `Button` props, plus `formId?` to target a form by `id` | Submit button with a loading state while submitting |
| `FormErrorBanner` | None | Lists the form's errors. `FormDialog` includes one |
| `AddButton`     | `label?`, `shape?`, `onClick` | The "add item" button used by the array layouts |

Hooks must be called inside a `Form`.

## Read-Only Display Forms

Custom detail pages often show record values as read-only fields. They wrap the fields in a `Form` with a no-op `onSubmit`. From `config/entities/run/run-info-page.tsx`:

```tsx
<Form
  onSubmit={() => undefined}
  initialValues={Object.fromEntries(fields.map((field) => [field.id, field.value]))}
>
  {fields.map((field) => (
    <StringField key={field.id} name={field.id} label={field.label} readOnly />
  ))}
</Form>
```

## Example

Trimmed from `config/entities/targets/create-inference-server-form.tsx`, which is the `createAction` component for the `targets` entity:

```tsx
import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { NumberField } from '#core/components/form/fields/number/number-field';
import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { FormRow } from '#core/components/form/layout/form-row/form-row';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { maxLength, min, regex } from '#core/components/form/validation/validators';

// onClose comes from the component's props; handleCreate, initialValues and
// BACKEND_TYPE_OPTIONS are defined elsewhere in the source file.
<FormDialog
  isOpen
  onDismiss={onClose}
  heading="Create target"
  onSubmit={handleCreate} // calls createMutation.mutateAsync(...)
  submitLabel="Create"
  initialValues={initialValues}
>
  <StringField
    name="metadata.name"
    label="Name"
    required
    maxLength={K8S_NAME_MAX_LENGTH}
    validate={combineValidators(
      maxLength(K8S_NAME_MAX_LENGTH),
      regex(K8S_NAME_PATTERN, K8S_NAME_RULES_MESSAGE)
    )}
    caption={K8S_NAME_RULES_MESSAGE}
    placeholder="e.g. my-inference-server"
  />

  <SelectField
    name="spec.backendType"
    label="Backend type"
    required
    options={BACKEND_TYPE_OPTIONS}
    clearable={false}
  />

  <FormGroup
    title="Initialization"
    description="Applied once when the server is provisioned on each cluster target."
  >
    <FormRow>
      <NumberField name="spec.initSpec.resourceSpec.cpu" label="CPU cores" required validate={min(1)} />
      <NumberField name="spec.initSpec.resourceSpec.gpu" label="GPUs" validate={min(0)} />
    </FormRow>
  </FormGroup>
</FormDialog>
```

The real file also repeats `required()` inside each `validate`; it's dropped here because `required` already adds it. `K8S_NAME_MAX_LENGTH`, `K8S_NAME_PATTERN` and `K8S_NAME_RULES_MESSAGE` come from `#core/utils/crd-utils`.

## Source Files

**Type definitions:**
- [javascript/packages/core/components/form/types/form-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/types/form-types.ts) - `FormProps`, `FormState`, `ArrayFieldOptions`
- [javascript/packages/core/components/form/components/form-dialog/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/components/form-dialog/types.ts) - `FormDialogProps`
- [javascript/packages/core/components/form/fields/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/fields/types.ts) - `BaseFieldProps`
- `javascript/packages/core/components/form/fields/<name>/types.ts` - per-field props
- [javascript/packages/core/components/form/layout/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/layout/types.ts) - `ArrayLayoutProps`; per-layout props in `layout/<name>/types.ts`
- [javascript/packages/core/components/form/validation/validators.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/validation/validators.ts) - validators

**Behavior:**
- [javascript/packages/core/components/form/hooks/use-field.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/hooks/use-field.ts) - `required` adds the `required()` validator
- [javascript/packages/core/components/form/components/form-dialog/form-dialog.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/form/components/form-dialog/form-dialog.tsx) - auto-close and error handling
- [javascript/packages/core/index.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/index.tsx) - what the package exports

**Real examples:**
- [javascript/packages/core/config/entities/targets/create-inference-server-form.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/targets/create-inference-server-form.tsx) - `FormDialog`, `FormGroup`, `FormRow`, validators
- [javascript/packages/core/config/entities/deployment/deployment-form.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/deployment/deployment-form.tsx) - one form shared by create and update, with `readOnly` in update mode
- [javascript/packages/core/config/entities/pipeline/create-pipeline-run-form.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/pipeline/create-pipeline-run-form.tsx) - custom action modal

## Unsupported: Declarative Forms

This section is for contributors working on `packages/core`; it isn't part of the supported API.

`packages/core` also contains a declarative form layer: `FormConfig` (`fields` + `layout`), `LayoutItemList`, `ConfigDrivenForm` and `FormProvider` for custom renderers. **It isn't a supported surface.**
- None of it is exported from `@michelangelo-ai/core`.
- Only the sandbox view (`components/views/sandbox/`) and tests use it.
- No production form uses it.

The `FieldConfigExtensions` interface in `config-types.ts` is declared for module augmentation, but nothing reads it, so augmenting it has no effect. Some form pieces exist only for this layer and aren't exported either: `MapField`, `FormGrid`, `FormNote` and the conditional layout (`layout/condition/`).

## Related Documentation

- [Action Configuration Reference](./action-configuration-reference.md) - Opening a form from a custom modal or `createAction`
- [Detail View Configuration Reference](./detail-view-configuration-reference.md) - Custom pages that host read-only forms
- [Cell Type Reference](./cell-type-reference.md) - Cells that render values in tables and detail headers
- [UI Components](../ui-components.md) - Other shared components
