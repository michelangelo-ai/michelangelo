# require-cast-comment

## What this rule enforces

Every `as` type assertion must carry a `// cast: <reason>` comment. `as const` and `as unknown` are exempt, since neither narrows a type in a way that can be wrong.

```ts
// ✗ Bad — no justification, can't tell if this is safe or a bug waiting to happen
const value = response as UserRecord;

// ✓ Good — states why the assertion is safe
// cast: response.json() returns any; shape is asserted, not validated
const value = response as UserRecord;
```

## Why a comment, not a ban

This isn't about eliminating every `as` — sometimes the fully type-safe version of some code is harder to follow. It's about understanding: an unexplained cast reads the same whether it's a real workaround or a bug, and that's exactly the context that erodes as the surrounding code changes.

## If this just failed your push

Work through these before writing `// cast:`:

1. **Can it go away?** Assume the assertion isn't necessary and try to remove it
2. **Is this a real gap in code you're not touching right now?** File a tracked issue and reference it (`see #1234`).
3. **Is this a permanent boundary?** A third-party library's looser types, or something TypeScript genuinely can't express — say so plainly.

## Back it with evidence

The strongest version of a comment links something checkable: this repo's tracking issue for a deferred gap, or an upstream TypeScript/library GitHub issue if the cast exists because of a bug or limitation someone else already filed.

```ts
// ✗ Bad — a bare assertion, no way to check it
// cast: always the entity object
const entityData = data[key] as Record<string, unknown>;

// ✓ Good — links the actual limitation
// cast: config carries no generic for this key's shape; see #1425
const entityData = data[key] as Record<string, unknown>;
```

## What NOT to include

- Filler that states a cast exists without saying why ("type assertion needed here").
- A comment on `as const` or `as unknown` — the rule doesn't require one, and it wouldn't add information either.
