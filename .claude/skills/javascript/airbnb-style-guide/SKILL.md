---
name: Airbnb JavaScript Style Guide
description: "Apply JavaScript best practices, conventions, and style rules from Airbnb's JavaScript Style Guide. Use when writing, reviewing, or refactoring JavaScript/TypeScript code to ensure clean, maintainable, and modern implementations."
---

# Airbnb JavaScript Style Guide

Apply best practices and conventions from the [Airbnb JavaScript Style Guide](https://github.com/airbnb/javascript) to write clean, idiomatic JavaScript code.

## When to Apply

Use this skill automatically when:
- Writing new JavaScript/TypeScript code
- Reviewing JavaScript/TypeScript code
- Refactoring existing JavaScript implementations

## Key Reminders

Follow the conventions and patterns documented at https://github.com/airbnb/javascript, with particular attention to:

### Variables and References

- **Use `const`** for all references; avoid using `var`
- **Use `let`** if you must reassign references
- `const` and `let` are block-scoped, unlike `var` which is function-scoped
- One variable per declaration: `const foo = 1; const bar = 2;` ✅

### Objects

- Use literal syntax for object creation: `const obj = {};` ✅
- Use shorthand for object methods: `{ getValue() { } }` ✅
- Use property value shorthand: `{ name }` instead of `{ name: name }`
- Group shorthand properties at the beginning of the object
- Only quote properties that are invalid identifiers
- Use object spread operator `...` for shallow copies: `{ ...obj, key: value }`

### Arrays

- Use literal syntax: `const arr = [];` ✅
- Use `Array.push()` instead of direct assignment
- Use array spreads `...` to copy arrays: `[...arr]`
- Use `Array.from()` to convert array-like objects to arrays
- Use return statements in array method callbacks: `map`, `filter`, etc.

### Destructuring

- **Use object destructuring** for multiple properties: `const { firstName, lastName } = user;`
- **Use array destructuring**: `const [first, second] = arr;`
- Use object destructuring for function parameters with multiple returns

### Strings

- **Use single quotes `''`** for strings (or template literals)
- Use template literals for string concatenation: `` `Hello ${name}` `` ✅
- Never use `eval()` on a string - it opens security vulnerabilities
- Don't unnecessarily escape characters in strings

### Functions

- **Use named function expressions** instead of function declarations
- Use arrow functions for anonymous functions/callbacks
- Arrow function implicit returns: `arr.map(x => x * x)`
- Use default parameter syntax: `function foo(opts = {}) { }`
- Avoid using `arguments`, use rest syntax `...args` instead
- Never mutate parameters
- Put default parameters last

### Arrow Functions

- **Always use parentheses** around arguments for clarity: `(x) => x * x` ✅
- Avoid confusing arrow function syntax with comparison operators
- Use implicit return when appropriate: `arr.map(x => x * x)`
- Use explicit return for multi-line bodies with braces

### Classes & Constructors

- Always use `class` syntax, avoid manipulating `prototype` directly
- Use `extends` for inheritance
- Methods can return `this` to help with method chaining
- Write a custom `toString()` method when appropriate
- Classes have a default constructor if one isn't specified
- Avoid duplicate class members

### Modules

- **Always use modules** (`import`/`export`) over non-standard module systems
- Use named exports for utilities, default exports for single responsibility
- Import from a path in only one place: `import { foo, bar } from './module';`
- Don't export mutable bindings
- Prefer default export when there's only one export
- Put all `import`s above non-import statements
- Multiline imports should be indented like arrays/objects

### Iterators and Generators

- **Don't use iterators** - prefer JavaScript's higher-order functions
- Use `map()`, `reduce()`, `filter()`, `find()`, `some()`, `every()` instead of `for-of`
- Use generator functions `function*` when appropriate
- Space generators properly: `function* generator() { }`

### Properties

- Use dot notation `.` when accessing properties
- Use bracket notation `[]` when accessing properties with a variable
- Use `**` for exponentiation: `2 ** 10` ✅

### Variables

- Always use `const` or `let` to declare variables
- One `const` or `let` per variable declaration
- Group `const`s and then `let`s
- Assign variables where you need them (reasonable scope)
- Don't chain variable assignments: `let a = b = c = 1;` ❌

### Comparison Operators & Equality

- **Use `===` and `!==`** over `==` and `!=`
- Conditional statements evaluate using `ToBoolean` abstract method
- Shortcuts for booleans, explicit comparisons for strings/numbers
- Use braces for multi-line blocks in case statements with declarations

### Blocks

- Use braces with all multi-line blocks
- Put `else` on the same line as `if` block's closing brace
- No `else` after `return` in `if` block (use early return pattern)

### Control Statements

- Don't use selection operators in place of control statements
- Multi-line control statements should be indented and wrapped

### Comments

- Use `/** ... */` for multi-line comments (JSDoc style)
- Use `//` for single-line comments
- Prefix with `FIXME` or `TODO` to help other developers
- Start comments with a space for readability
- Use `// FIXME:` to annotate problems
- Use `// TODO:` to annotate solutions to problems

### Whitespace

- Use soft tabs (spaces) set to 2 spaces
- Place 1 space before leading brace: `function test() {`
- Place 1 space before opening parenthesis in control statements
- No space between function name and parentheses: `function foo() {`
- Set off operators with spaces: `const x = y + 5;`
- End files with a single newline character
- Use indentation for long method chains (more than 2 chained methods)
- Leave a blank line after blocks and before the next statement

### Commas

- **No leading commas** ❌
- **Add trailing commas** in multi-line: ✅

### Semicolons

- **Always use semicolons** ✅
- Never rely on automatic semicolon insertion (ASI)

### Type Casting & Coercion

- Perform type coercion at the beginning of the statement
- Strings: `String(value)` or template literals
- Numbers: `Number(value)` or `parseInt(value, 10)`
- Booleans: `Boolean(value)` or `!!value`

### Naming Conventions

- Avoid single letter names; be descriptive: `getUserData()` ✅
- **Use camelCase** for objects, functions, instances: `thisIsMyObject`
- **Use PascalCase** for classes and constructors: `class User { }`
- **Use UPPER_SNAKE_CASE** for constants: `const API_KEY = 'abc123';`
- Don't use trailing or leading underscores (private convention is discouraged)
- Don't save references to `this` - use arrow functions or `.bind()`
- Base filename should match default export name
- Use PascalCase for acronyms: `HttpRequest` not `HTTPRequest`

### Accessors

- Accessor functions for properties are not required
- If you make accessor functions, use `getVal()` and `setVal('hello')`
- If the property is a boolean, use `isVal()` or `hasVal()`
- It's okay to create `get()` and `set()` functions, but be consistent

### Events

- When attaching data to events, pass an object instead of raw values
- This allows subsequent contributors to add more data without finding/updating every handler

### jQuery (if used)

- Prefix jQuery object variables with `$`: `const $sidebar = $('.sidebar');`
- Cache jQuery lookups
- Use cascading for jQuery operations
- Use `find` with scoped jQuery queries: `$('.sidebar ul')` or `$sidebar.find('ul')`

### ECMAScript Standards

- Prefer ES6+ features when available
- Use template literals over string concatenation
- Use destructuring, spread operators, default parameters
- Use arrow functions for callbacks
- Use `async`/`await` over raw promises when possible
- Use `const` and `let` - never use `var`

## Anti-Patterns to Avoid

- ❌ Using `var` instead of `const`/`let`
- ❌ Using `==` instead of `===`
- ❌ Mutating function parameters
- ❌ Reassigning `const` references
- ❌ Using `eval()` or `with` statements
- ❌ Creating functions inside loops
- ❌ Modifying built-in prototypes
- ❌ Using `arguments` instead of rest parameters
- ❌ Unnecessary function binding
- ❌ Nested ternaries that reduce readability

## References

- Official Guide: https://github.com/airbnb/javascript
- ESLint Config: https://www.npmjs.com/package/eslint-config-airbnb
- React/JSX Style Guide: https://github.com/airbnb/javascript/tree/master/react
- CSS-in-JavaScript: https://github.com/airbnb/javascript/tree/master/css-in-javascript
