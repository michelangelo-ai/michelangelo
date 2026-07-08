# require-cast-comment

## What this rule enforces

Every `as` type assertion must carry a `// cast: <reason>` comment — on the same line, or anywhere in the unbroken `//` comment block on the line(s) immediately above. `as const` and `as unknown` are exempt, since neither narrows a type in a way that can be wrong.

```ts
// ✗ Bad — no justification, can't tell if this is safe or a bug waiting to happen
const value = response as UserRecord;

// ✓ Good — states why the assertion is safe
const value = response as UserRecord; // cast: response.json() returns any; shape is asserted, not validated
```

## Why a comment, not a ban

An `as` assertion turns off the type checker for that value. That's sometimes the right call — no ban would survive contact with real code, since libraries and runtime boundaries are imprecise in ways TypeScript can't express. But type assertions can be escape hatches for real issues, and their justification can lose context over time as the surrounding code changes. An unexplained cast reads the same whether it's genuinely unavoidable or quietly wrong, and nothing forces the author to say which. Requiring a comment doesn't fix casts — it forces a decision at the point of use, one a reviewer can actually evaluate instead of taking on faith.

## Work through these in order before writing the comment

1. **Can the cast be removed outright?** Check it against the type checker, not just against what looks plausible — an existing typed overload, a missing null check, or a stale API shape often make a cast unnecessary. If it's not needed, delete it. Don't justify what you can delete.
2. **Can the underlying issue be fixed cheaply?** If the cast is propping up a real type mismatch, fix the type instead of re-justifying the cast. A cast hiding a bug is worse than the bug, since it also disables the compiler in that spot from then on.
3. **Is this a real gap in foundational code you can't fix right now?** This is the case to watch for. If the cast exists because of a limitation upstream — a type that's too loose, a generic that isn't threaded through, a design decision out of scope for what you're doing — don't paper over it with a confident-sounding comment. File a tracked issue describing the actual gap and reference it in the comment (`// cast: ...; see #1234`). Deferring the fix is fine. Deferring it silently is not.
4. **Is this a genuine, permanent boundary?** Some casts will never go away: a third-party library types something looser than it behaves, or TypeScript can't express the relationship (e.g., it can't correlate a generic against itself from inside its own implementation). Say so plainly — that's the one case where the comment is the whole fix, not a placeholder for one.

## Write what's actually known, not what sounds reassuring

"Always returns X" and "verified above" are claims — write them only if you checked, not because they're the words that make a cast look settled.

```ts
// ✗ Bad — asserts a guarantee that was never checked
const entityData = data[key] as Record<string, unknown>; // cast: always the entity object

// ✓ Good — states what's actually known, and defers what isn't
const entityData = data[key] as Record<string, unknown>; // cast: config carries no generic for this key's shape; see #1425
```

A comment that turns out to be wrong is worse than an unexplained cast — it misleads the next reader instead of just leaving a gap they'd know to check.

## Multi-line comments

If the reason doesn't fit on one line under the project's `printWidth`, wrap it into a leading comment block instead of cramming everything onto one line:

```ts
// cast: our ColumnMeta augmentation is an empty interface (TS can't have it extend the
// Cell<TData> union); always ColumnConfig<T> per our column setup; see #1417
column: cell.column.columnDef.meta! as ColumnConfig<T>,
```

The `cast:` marker can appear anywhere in the unbroken block directly above the assertion — it doesn't have to be on the line touching the code. A blank line still breaks the connection to the assertion below it.

## What NOT to include

- Filler that states a cast exists without saying why ("type assertion needed here").
- A comment on `as const` or `as unknown` — the rule doesn't require one, and it wouldn't add information either.
