# CLAUDE-COMMITS.md - Commit Message Guidelines

USE PROACTIVELY to write commit messages that explain WHY changes were made,
not HOW they work. Code is generally self-explanatory for implementation details.

## Commit Writing Flow

1. Begin by aligning with the developer on particular changes to consider when
   writing the commit. Encourage the developer to stage changes ready to commit
   or provide git SHA(s) that include changes

2. Read each file that was changed and summarize the changes.

3. Write a commit message following the guidance outlined within this file,
   CLAUDE-COMMITS.md

## Core Principles

- Focus on the reasons why the change was made
- Explain what was wrong before, how it works now, and why this solution was chosen
- Group related code changes and add line breaks between distinct changes
- Be concise and not persuasive
- Leave out implementation details - code explains the "how"
- Be quick to ask clarifying questions of the developer

## Structure Template

```
Summarize the change in around 50 characters

Action-oriented phrase that describes what was done at a high level.
Use phrases like "Replace X with Y" or "Migrate from X to Y". Wrap
commit body lines at around 72 characters.

Explain the reasoning behind why the code was changed in the ways
it was changed. For example, explain what was wrong with the previous
approach and why it needed to change. Describe any architectural
constraints or limitations that drove this decision.

Explain how the new approach solves the problem and why this specific
solution was chosen over alternatives. Include any side effects or
consequences that aren't immediately obvious.
```

## Situational Guidelines

**Variable/Function Changes:**

- Don't detail variable name changes or new function exports
- Focus on why the function was needed and how it solves the problem
- For new functions, explain the problem they solve, not where they're used

**Configuration/Build Changes:**

- Don't detail exact config changes
- Explain why the change was needed and how it helps solve the broader problem

**Bug Fixes:**

- Explain the bug's root cause
- Describe how this change fixes the underlying issue
- Don't just describe symptoms

**Refactoring:**

- Explain why refactoring was necessary
- Describe what architectural problem it solves
- Connect it to enabling future capabilities (e.g., "blocks React hooks integration")

**Constant/Value Changes:**

- Don't detail the exact value change
- Focus on why the change to the constant was needed

## What NOT to Include

- Details about how code works (code is self-explanatory)
- Variable name changes as standalone items
- Exact configuration file modifications
- Changes related to testing or building
- Step-by-step implementation details
- Where functions are imported/exported
- Persuasive language about code quality

## Example Problem-Solution Pattern

"Previous approach used inheritance pattern with filterFactory, which blocked
React hooks architecture since hooks cannot use class inheritance. This
change consolidates filtering into a single hook that automatically detects
column configuration, eliminating the factory dependency and enabling future
integration with React context for cellToString functionality."

## Chat History Context

Since Claude has access to chat history, reference previous conversations
that explain the architectural motivations and constraints that led to
the change.

### Determining changes to consider

During the course of a Claude Code Chat, many code changes may be discussed. You
should always verify what the final changes were accepted by the developer.

There also may be changes that were not discussed in any Claude Chat. For these
changes, assess the changes in the context of the Claude Chat but do not assume
that a clear connection will exist.
