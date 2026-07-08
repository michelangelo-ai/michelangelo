# require-cast-comment

## What this rule enforces

Every `as` type assertion must carry a `// cast: <reason>` comment, in the unbroken `//` comment block on the line(s) immediately above it. `as const` and `as unknown` are exempt, since neither narrows a type in a way that can be wrong.

```ts
// ✗ Bad — no justification, can't tell if this is safe or a bug waiting to happen
const value = response as UserRecord;

// ✓ Good — states why the assertion is safe
// cast: response.json() returns any; shape is asserted, not validated
const value = response as UserRecord;
```

## Why a comment, not a ban

This isn't about eliminating every `as` — sometimes the fully type-safe version of some code is harder to follow, not easier, and a cast with a reason is the more maintainable outcome. It's about understanding: an unexplained cast reads the same whether it's a real workaround or a bug, and that's exactly the context that erodes as the surrounding code changes. The comment is what's left once the reasoning in your head is gone.

## If this just failed your push

Work through these before writing `// cast:`:

1. **Can it go away?** Check against the type checker, not against what looks plausible — a missing null check, a stale type, or an existing overload often means the cast can just be deleted.
2. **Is this a real gap in code you're not touching right now?** File a tracked issue and reference it (`see #1234`). Deferring the fix is fine; deferring it without a paper trail isn't.
3. **Is this a permanent boundary?** A third-party library's looser types, or something TypeScript genuinely can't express — say so plainly. That's the one case where the comment alone is the fix.

## Back it with evidence

"Always returns X" and "verified above" are claims — worth writing only if you checked. The strongest version of a comment links something checkable: this repo's tracking issue for a deferred gap, or an upstream TypeScript/library GitHub issue if the cast exists because of a bug or limitation someone else already filed.

```ts
// ✗ Bad — a bare assertion, no way to check it
// cast: always the entity object
const entityData = data[key] as Record<string, unknown>;

// ✓ Good — links the actual limitation
// cast: config carries no generic for this key's shape; see #1425
const entityData = data[key] as Record<string, unknown>;
```

If there's no issue to link, that's usually a sign one should be filed.

## Comments can span multiple lines

The block above the assertion can be as long as it needs to be — wrap it like any other comment instead of cramming the reason onto one line:

```ts
// cast: our ColumnMeta augmentation is an empty interface (TS can't have it extend the
// Cell<TData> union); always ColumnConfig<T> per our column setup; see #1417
column: cell.column.columnDef.meta! as ColumnConfig<T>,
```

The `cast:` marker can appear anywhere in the unbroken block — it doesn't have to be on the line touching the code. A blank line breaks the connection to the assertion below it, so the block has to be contiguous.

## What NOT to include

- A trailing same-line comment (`foo as Bar; // cast: ...`) — the rule only looks above the assertion, not beside it.
- Filler that states a cast exists without saying why ("type assertion needed here").
- A comment on `as const` or `as unknown` — the rule doesn't require one, and it wouldn't add information either.
