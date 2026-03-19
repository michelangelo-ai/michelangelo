# confirm-dialog

## Purpose

A modal dialog component for confirming a user-initiated action, with built-in async loading state, error display on failure, and auto-close on success.

## Exports

| Export | Kind | Availability | Description |
|--------|------|--------------|-------------|
| `ConfirmDialog` | React component (`React.FC<ConfirmDialogProps>`) | `packages/core` (internal) | The confirm dialog component |
| `ConfirmDialogProps` | TypeScript interface | `packages/core` (internal) | Prop types for `ConfirmDialog` |

There is no barrel (`index.ts`) — consumers import directly from source files per codebase convention.

## Props / Signatures

### `ConfirmDialog` / `ConfirmDialogProps`

| Prop | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| `isOpen` | `boolean` | — | Yes | Controls dialog open/close state |
| `onDismiss` | `() => void` | — | Yes | Called when Cancel is clicked or dialog is dismissed; also called automatically on successful confirm |
| `heading` | `string` | — | Yes | Dialog title rendered in the header |
| `onConfirm` | `() => Promise<void> \| void` | — | Yes | Async action to perform on confirmation; throw an `Error` to signal failure |
| `confirmLabel` | `string` | `'Confirm'` | No | Label text for the primary action button |
| `confirmButtonColor` | `string` | `undefined` | No | CSS background color for the confirm button (e.g. `theme.colors.backgroundNegative` for destructive actions) |
| `children` | `ReactNode` | `undefined` | No | Body content of the dialog |
| `size` | `Size` (from `baseui/dialog`) | `SIZE.small` | No | Controls dialog width |

## Used By

| Path | Reason |
|------|--------|
| `packages/core/components/views/sandbox/sandbox.tsx` | Developer sandbox demonstrating three variants: basic success, error-stays-open, and destructive red-button; each controls `isOpen` state locally and passes an inline `onConfirm` async function |
| `packages/core/components/modal/confirm-dialog/__tests__/confirm-dialog.test.tsx` | Unit tests covering all behavioral branches |

No production application code currently imports `ConfirmDialog` — it is available for consumers to use directly from `packages/core`.

## Test Coverage

- **File:** `__tests__/confirm-dialog.test.tsx`
- **Framework:** Vitest + React Testing Library (`@testing-library/react`, `@testing-library/user-event`)
- **Wrapper:** `buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])` — provides BaseUI theme and icon context required by the `Dialog` dependency

### Test cases

| Test | What it verifies |
|------|-----------------|
| Renders dialog with heading and buttons | ARIA role `dialog`, confirm label, and Cancel button are present when open |
| Renders body content as children | `children` appears inside the dialog |
| Default confirm label | `'Confirm'` is used when `confirmLabel` is omitted |
| Does not render when closed | `isOpen={false}` results in no `dialog` role in the DOM |
| Calls onConfirm and auto-closes on success | `onConfirm` is called once; `onDismiss` is called once (auto-close) |
| Calls onDismiss when cancel is clicked | Cancel button triggers `onDismiss` |
| Shows error and stays open on throw | Banner with thrown `Error.message` appears; `onDismiss` not called |
| Re-enables confirm button after error | Confirm button is not disabled after the async error resolves |
| Disables cancel while loading | Cancel is `disabled` during the in-flight async window |
| Custom confirmButtonColor | Confirm button has `backgroundColor: '#DE1135'` inline style |
| Clears error on reopen | Closing (`isOpen=false`) then reopening (`isOpen=true`) removes the previous error banner |

### Notable patterns

- The "does not render when closed" test uses a try/catch around `findByRole` with a 100 ms timeout rather than `queryByRole`, since the underlying `Dialog` may animate out asynchronously.
- The "disables cancel while loading" test holds the promise open via a manually controlled `resolve` reference, asserting the disabled state before releasing it — avoids any timing assumptions.

## Internal Dependencies

| Import | Source | Why |
|--------|--------|-----|
| `Dialog` | `#core/components/dialog/dialog` | Provides the modal shell (overlay, heading, `buttonDock`, placement, size) |
| `Button`, `KIND` | `baseui/button` | Primary (confirm) and dismissive (cancel) buttons |
| `Banner`, `KIND as BANNER_KIND` | `baseui/banner` | Inline error display inside the dialog body |
| `PLACEMENT`, `SIZE` | `baseui/dialog` | Constants for dialog position (`topCenter`) and size (`small` default) |
| `useEffect`, `useState` | `react` | Loading state, error state, and reset-on-close side effect |

## Edge Cases to Preserve

1. **State reset on close:** `isLoading` and `error` are reset via `useEffect` watching `isOpen`. A re-opened dialog always starts from a clean slate regardless of prior failure. This is load-bearing for the reopen test and for UX correctness.
2. **Cancel disabled during load:** The Cancel button is `disabled={isLoading}`, not hidden. This prevents double-dismiss while a slow `onConfirm` is in flight.
3. **Confirm button uses `isLoading` not `disabled`:** The confirm button passes `isLoading` to BaseUI's `Button`, which shows a spinner and prevents re-click internally. Do not replace this with `disabled`.
4. **Error extracted from `Error` instance:** The catch block checks `err instanceof Error` before reading `.message`; non-Error rejections fall back to `'An unexpected error occurred.'` This must be preserved for non-standard rejection values.
5. **`onDismiss` is not called on failure:** When `onConfirm` throws, only `setIsLoading(false)` is called — the dialog stays open. `onDismiss` is only called on success.
6. **`handleConfirm` passed directly as `onClick`:** The confirm button uses `onClick={handleConfirm}` — TypeScript allows this because React's `onClick: () => void` return type means "return value is not used", not "must return void", so an async function is assignable.
7. **Placement hardcoded to `topCenter`:** `ConfirmDialog` always renders at `PLACEMENT.topCenter`. This is not exposed as a prop — it is a design constraint for this component type.

## Implementation Decisions

### Async error handling via throw rather than a result type

**What:** `onConfirm` is typed as `() => Promise<void> | void`. Failure is signaled by throwing (or rejecting). There is no `{ ok, error }` result type.

**Why:** The component follows the `FormDialog` pattern already established in the codebase. Throwing keeps the caller's async function idiomatic (`await doThing()` rather than `const result = await doThing(); if (!result.ok) ...`).

**Root cause:** Consistency with `FormDialog`'s established error contract so callers behave identically across both dialog types.

**Port or simplify?:** Keep as-is. The throw-based contract is idiomatic, already tested, and consistent with the sibling `FormDialog`.

### Error state stored in component rather than propagated upward

**What:** `error` is held in local `useState`. The parent never sees it.

**Why:** Confirm dialogs are ephemeral; the parent only cares whether the action succeeded or failed (via `onDismiss` being called or not). Surfacing the error upward would require parents to manage and clear it — unnecessary coupling.

**Root cause:** The dialog's responsibility is to present the error in context, not to relay it. The parent already has `onConfirm` and knows what can fail.

**Port or simplify?:** Keep as-is.

### `confirmButtonColor` applied via BaseUI `overrides` rather than a typed color token

**What:** The destructive button color is passed as a raw CSS string and applied as an inline `style={{ backgroundColor: confirmButtonColor }}` on the BaseUI `Button`.

**Why:** Inline style is directly assertable in tests via `toHaveStyle`, unlike Styletron-generated class names which are not inspectable per-element in jsdom. A background color override doesn't need to interact with Styletron's cascade.

**Root cause:** BaseUI's `Button` overrides apply styles through Styletron (atomic CSS class injection), which `getComputedStyle` in jsdom cannot resolve. Inline style bypasses this and makes the value testable.

**Port or simplify?:** Keep as inline style. If hover/active state is ever needed for the destructive variant, switch to a typed `variant` enum and handle it via overrides with a `$theme` style function.

### State reset via `useEffect` watching `isOpen` rather than resetting in the dismiss handler

**What:** Reset of `isLoading` and `error` is driven by `useEffect([isOpen])` when `isOpen` becomes `false`, not by resetting inside `onDismiss`.

**Why:** The dialog can be closed from two places: the Cancel button (which calls `onDismiss`) and programmatic changes to `isOpen` from the parent. A single `useEffect` handles both uniformly.

**Root cause:** If reset were placed only in the cancel `onClick`, a parent that sets `isOpen={false}` for an external reason (e.g., navigating away) would leave stale error state on the next open.

**Port or simplify?:** Keep as-is. The effect is the correct reactive mechanism for this pattern.

## Migration Notes

- **Status:** Done
- **Target equivalent:** This IS the target — it lives in `packages/core`
- **Prerequisites:** None
- **Fix in port (non-negotiable):** No known bugs or workarounds are embedded in the implementation.
- **Simplification opportunities:** The `confirmButtonColor` prop accepts a raw CSS string. A future iteration could replace it with a typed variant enum (e.g., `'default' | 'destructive'`) that maps to design tokens internally, removing the raw color string from the public API.
