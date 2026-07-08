**What type of PR is this? (check all applicable)**
- [ ] Refactor
- [ ] Feature
- [ ] Bug Fix
- [ ] Optimization
- [ ] Documentation Update

<!-- Describe what has changed in this PR -->
**What changed?**


<!-- Tell your future self why have you made these changes -->
**Why?**


<!-- How have you verified this change? Tested locally? Added a unit test? Checked in staging env? -->
**How did you test it?**


<!-- Assuming the worst case, what can be broken when deploying this change to production? -->
**Potential risks**

<!-- Does this PR introduce a breaking change? Check all that apply. -->
**Breaking Changes**

- [ ] No breaking changes
- [ ] API changes (Go exported symbols or function signatures, Python public functions or classes)
- [ ] Proto changes (enum value renumbering, field number changes, field removal, service removal)
- [ ] Helm changes (new required values, renamed or removed keys, changed value semantics)
- [ ] Config/deployment changes (new required env vars, renamed container args, changed ports or mount paths)

> If any breaking change box is checked (other than "No breaking changes"), you **must**:
> 1. Use the `BREAKING CHANGE:` footer **or** the `!` suffix (e.g. `feat!:`) in your commit message — see [Commit Messages](../CONTRIBUTING.md#commit-messages).
> 2. Fill in the **Migration guide** section below with step-by-step upgrade instructions.

<!-- Required only if a breaking change box above is checked. Delete this section if no breaking changes. -->
**Migration guide**

_Describe the steps an operator or downstream consumer must take to upgrade. Include before/after examples for any API, config, or schema change._

<!-- Is it notable for release? e.g. schema updates, configuration or data migration required? If so, please mention it, and also update CHANGELOG.md -->
**Release notes**

<!-- Does this PR introduce a user-facing or API change? Is user document updated? Is the change backward compatible? If so please update in wiki https://github.com/michelangelo-ai/michelangelo/wiki? -->
**Documentation Changes**
