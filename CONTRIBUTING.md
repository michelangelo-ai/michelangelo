# Contributing to Project

Thanks for taking the first step in contributing to our project.

**Uber welcomes contributions of all kinds and sizes. This includes everything from simple bug reports to large features.** 

See the [Table of Contents](#table-of-contents) for different ways to contribute and details about how we treat each contribution. Please read the relevant section before making your contribution as it will not only make it a lot easier for us but also ensure you have the very best developer experience too.

>:star: If you like the project, but don't have time to contribute just now, that's no problem at all!. 

There are other easy ways to help support the project and show your appreciation including
* Star the project
* Join the our community
* Shout about us online or at local meetups with your peers & colleagues

<a id="table-of-contents"></a>
## Table of Contents
- [Code of Conduct](#code-of-conduct)
- [I Have a Question](#i-have-a-question)
- [I Want To Contribute](#i-want-to-contribute)
  - [Legal Notice](#legal-notice)
- [Enhancements and Features](#enhancements-and-features)
  - [Before Submitting an Enhancement or Feature](#before-submitting-an-enhancement-or-feature)
  - [How do I submit a Good Enhancement or Feature](#how-do-i-submit-a-good-enhancement-or-feature)
- [Reporting Bugs](#reporting-bugs)
  - [Before Submitting a Bug Report](#before-submitting-a-bug-report)
  - [How Do I Submit a Good Bug Report?](#how-do-i-submit-a-good-bug-report)
- [Commit Messages](#commit-messages)
- [Creating a Pull Request](#creating-a-pull-request)
  - [Before Creating a Pull Request](#before-creating-a-pull-request)
  - [How Do I Submit a Good Pull Request?](#how-do-i-submit-a-good-pull-request)
- [Deprecation Policy](#deprecation-policy)

<a id="code-of-conduct"></a>
## Code of Conduct
This project and everyone participating in it is governed by our [Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you are expected to uphold these standards. 

<a id="i-have-a-question"></a>
## I Have a Question

> Please don't file an issue to ask a question. You'll get faster results by using the resources below.

If you want to ask a question about the project, there are a few options available to you.

* Check and read our [Documentation](https://michelangelo-ai.org/)
* Search our existing [Issues](https://github.com/michelangelo-ai/michelangelo/issues) as this may also help you.
* Join our Community (coming soon) to engage with other users and contributors,

If you’re still facing issues and need further help, then we recommend the following process:
* Open an [issue](https://github.com/michelangelo-ai/michelangelo/issues/new/choose).
* Provide as **much context as you can** about what you're running into.
* Provide any relevant platform versions (nodejs, npm, etc), depending on what seems related and **feel free to include screenshots or code-snippets**.

The project maintainers will then take care of the issue as soon as possible and help to resolve your question.

<a id="i-want-to-contribute"></a>
## I Want To Contribute

<a id="legal-notice"></a>
#### Legal Notice

When contributing to any Uber Open Source project, you agree that you have authored 100% of the content and that you have the necessary rights to that content and that the content you contribute may be provided under the project license. 

You’re required to sign our [Contributor License Agreement](https://cla-assistant.io/michelangelo-ai) to confirm this and you’ll be prompted to do this when submitting your first contribution.

## Our Contribution Tracks

To keep our development moving fast while protecting the stability of the platform, Michelangelo uses a **Dual-Track Pipeline**. Before you open an issue or write any code, please match your goal to the correct track:

* **Track 1: Maintenance:** For everyday bug fixes, code optimizations, testing improvements, or documentation updates. (This repo handles Track 1 entirely via standard Issues and Pull Requests).
* **Track 2: Evolution:** For major architectural changes, public API modifications, new core modules, or substantial new framework dependencies. (These require a design proposal in our dedicated enhancements repository *before* any code is written here).

*Note on Ecosystem Tools:* Backward-compatible updates, minor version bumps, and bug fixes to deployment configurations (like Helm charts) fall under Track 1. Breaking changes or entirely new deployment patterns belong in Track 2.

## Working on Track 1: Maintenance (Standard PR Loop)

If your contribution falls under Track 1, you will operate entirely within this repository using the standard sequence: **Open an Issue ➔ Submit a Pull Request ➔ Review & Merge**.

<a id="enhancements-and-features"></a>
## Enhancements and Features

This section guides you through submitting an enhancement or new functionality into the project; as well as minor improvements to existing functionality. Following these guidelines will help the community to understand your submission.

<a id="before-submitting-an-enhancement-or-feature"></a>
### Before Submitting an Enhancement or Feature

* Make sure that you are using the latest version of the project.
* **Read the [documentation](https://michelangelo-ai.org/) carefully** and find out if the functionality you’re proposing is already covered, this may well be through configuration.
* Perform a [search](https://github.com/michelangelo-ai/michelangelo/issues) to see if the enhancement has already been suggested. If it has, add a comment to the existing issue instead of opening a new one.
* Consider whether your **idea fits with the scope and aims of the project** and keep in mind that we want features that will be useful to the majority of our users and not just a handful.

<a id="how-do-i-submit-a-good-enhancement-or-feature"></a>
### How do I submit a Good Enhancement or Feature

Enhancements and new features suggestions are tracked as [issues](https://github.com/michelangelo-ai/michelangelo/issues).

* Open an [issue](https://github.com/michelangelo-ai/michelangelo/issues/new/choose).
* Use a **clear and descriptive title** for the issue to identify the suggestion.
* Provide a **description of the enhancement** with as many details as possible touching on what specifically is missing, out of date, wrong, or needs improvement.
* **Describe the current behaviour** of the project and **explain which behaviour you expected to see** instead and why. 
* You’re welcome to **include screenshots** which help you demonstrate the steps or point out which part your submission is related to.
* **Explain why this enhancement would be useful** to the majority of our project users. You may also want to point out the other projects that solved it better and which could serve as inspiration for making our tool even stronger.

<a id="reporting-bugs"></a>
## Reporting Bugs

This section guides you through submitting a Bug Report into the project where a behaviour or functionality isn’t working as you’d expected. Following these guidelines will help the community to understand your submission and ensure you’ve identified a bug correctly.

<a id="before-submitting-a-bug-report"></a>
### Before Submitting a Bug Report

Bug reports shouldn't need the project maintainers to clarify or search for more information. Therefore, we ask you to investigate carefully, collect information and describe the issue in detail in your report. If you complete the following steps in advance, then this will help us fix the issue as fast as possible.

* Make sure that you are using the latest version of the project.
* **Determine if your bug is really a bug** and not an error on your side e.g. using incompatible environment components/versions.
* To see if other users have experienced (and potentially already solved) the same issue you are having, **check if there is not already a bug report** existing for your bug or error in the [bug list](https://github.com/michelangelo-ai/michelangelo/issues?q=is%3Aissue%20state%3Aopen%20label%3Abug). If it has and the issue is still open, add a comment to the existing issue instead of opening a new one
* Collect information about the bug:
  *  OS, Platform and Version (Windows, Linux, macOS, x86, ARM)
  * Version of the interpreter, compiler, SDK, runtime environment, package manager, depending on what seems relevant – for local instances only.
  * If possible, your input and the output
  * Can you reliably reproduce the issue? 

<a id="how-do-i-submit-a-good-bug-report"></a>
### How Do I Submit a Good Bug Report?

> :warning: You must never report security related issues, vulnerabilities or bugs to the issue tracker, or elsewhere in public. Instead sensitive bugs should be submitted through the Uber [HackerOne](https://hackerone.com/uber) process.

We use GitHub issues to track bugs. If you run into an issue with the project:

* Open an [issue](https://github.com/michelangelo-ai/michelangelo/issues/new/choose) selecting the bug report template.
* Explain the behaviour you would expect and the actual behaviour.
* Please **provide as much context as possible** and describe the reproduction steps that someone else can follow to recreate the issue on their own.
* If you’re making changes to the project then your context should also include your code. For good bug reports **you should isolate the problem and create a reduced test case**.

### Once it's filed:

* The project team will label the issue accordingly.
* A project maintainer will try to reproduce the issue with your provided steps. If there are no reproduction steps, or no obvious way to reproduce the issue, we’ll request these details but the bug won’t be addressed until they are provided.
* If the team is able to reproduce the issue, it will be tagged and the issue will be queued to be implemented.

<a id="commit-messages"></a>
## Commit Messages

This project uses the [Conventional Commits](https://www.conventionalcommits.org/) specification. All commits to the repository must follow this format. Commit messages feed the automated changelog generation (via git-cliff) and drive semantic version bumps in the release pipeline.

### Format

```
type(scope): short description

Optional longer body explaining the motivation or detail.

BREAKING CHANGE: description of the breaking change (if applicable)
```

- **type** — required; describes the kind of change (see table below)
- **scope** — optional; identifies the component affected (see table below)
- **short description** — required; imperative mood, no capital first letter, no trailing period, ≤72 chars total for the subject line
- **body** — optional; separated from the subject by a blank line
- **BREAKING CHANGE footer** — required when the change breaks backwards compatibility

### Allowed types

| Type | When to use |
|---|---|
| `feat` | A new feature visible to users or downstream consumers |
| `fix` | A bug fix |
| `docs` | Documentation changes only |
| `ci` | CI/CD pipeline or workflow changes |
| `chore` | Build process or tooling changes with no production code change |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test` | Adding or updating tests |
| `perf` | Performance improvement |

### Allowed scopes

| Scope | Component |
|---|---|
| `python` | Python SDK / PyPI package |
| `helm` | Helm chart |
| `ci` | CI workflows and scripts |
| `npm` | npm / JavaScript package |
| `ui` | UI container |
| `go` | Go services / containers |

Scope may be omitted when a change is truly cross-cutting and does not belong to a single component.

### Examples

Simple feature:
```
feat(python): add async client support
```

Bug fix with scope:
```
fix(helm): correct default resource limits
```

Cross-cutting docs change (no scope):
```
docs: update quickstart guide
```

CI change:
```
ci(go): pin golangci-lint to v1.57.2
```

Tooling update (no scope):
```
chore: bump dev dependencies
```

Feature with a breaking change:
```
feat(python): remove deprecated v1 API

The v1 client module has been removed to reduce maintenance burden.
Migrate all callers to the v2 client before upgrading.

BREAKING CHANGE: The `michelangelo.v1` module is no longer available.
Replace all imports of `michelangelo.v1` with `michelangelo.v2`.
```

### BREAKING CHANGE convention

Add a `BREAKING CHANGE:` line in the commit footer (after a blank line separating it from the body) to signal a backwards-incompatible change. The release tooling will:

1. Include the commit in the **BREAKING CHANGES** section of the generated changelog.
2. Trigger a **major** version bump when the change ships in a release.

<a id="creating-a-pull-request"></a>
## Versioning & Tag Format

All release artifacts share the same **Major.Minor** version number. Patch versions may differ when a component-specific fix ships independently.

### Git tag format

| Release type | Tag example | Notes |
|---|---|---|
| Stable | `v0.3.0` | Standard SemVer — `vMAJOR.MINOR.PATCH` |
| Release candidate | `v0.3.0-rc.1` | Pre-release suffix per SemVer §9 |
| Nightly | `v0.3.0-nightly.20260624` | Date-stamped; not manually created |

### PEP 440 mapping (Python / PyPI)

Git tags use SemVer, but PyPI requires [PEP 440](https://peps.python.org/pep-0440/). The release pipeline translates automatically:

| Git tag | PyPI version |
|---|---|
| `v0.3.0` | `0.3.0` |
| `v0.3.0-rc.1` | `0.3.0rc1` |
| `v0.3.0-nightly.20260624` | `0.3.0.dev20260624` |

### Rules

- Tags are only created on **release branches** (`release/vX.Y`), never directly on `main`.
- All artifacts (Python, npm, Go, Helm, containers) receive the same Major.Minor from the tag.
- See the [Versioning Policy](./docs/getting-started/roadmap.md#versioning-policy) for stability level guarantees (stable, beta, alpha).

## Creating a Pull Request

If you want to fix a bug or propose a new feature you’ll do this through creating a Pull Request.

<a id="before-creating-pull-request"></a>
### Before Creating a Pull Request

* Check if there is an [issue](https://github.com/michelangelo-ai/michelangelo/issues/new/choose) that highlights the same problem that you want to solve or that requests the same feature that you want to implement. If this is the case, then **remember to link the issue in your Pull Request**.
* You might also want to check if a similar [pull request](https://github.com/michelangelo-ai/michelangelo/pulls) has already been created.
* It’s always good practice to consider creating an issue before creating a Pull Request but for smaller changes we don’t mind if you omit this stage.

<a id="how-do-i-submit-a-good-pull-request"></a>
### How Do I Submit a Good Pull Request?

* Use a **clear and descriptive title** for the Pull Request.
* Follow this [Pull Request template](https://github.com/michelangelo-ai/michelangelo/blob/main/.github/pull_request_template.md).
* **Link the issue** related to this Pull Request, if present.
* Provide a **short description of the solution you proposed** in as many details as possible.
* **Use comments in the code** that you provide to give us more context to any code based submissions.

Thanks for contributing into our project.

## Working on Track 2: Evolution (The RFC Process)

If your proposal falls under Track 2, **please do not open a standard issue or code PR in this repository first**. Large features must secure architectural design consensus before code implementation begins.

1. Head over to the [Michelangelo Enhancements Repository](https://github.com/michelangelo-ai/enhancements).
2. Review the review SLAs, lifecycle stages, and copy the baseline design template (`rfcs/20260101-template.md`) to open a proposal Pull Request there.

### Implementing an Approved RFC

Once the enhancements process marks your proposal as **Accepted**, you will return here to implement the code under an operational agreement:

* **Tracking & Targeting:** Your assigned Shepherd will open a dedicated tracking issue in this repository to map individual code PRs and target them to an upcoming release milestone branch (e.g., `release-0.2`).
* **The Code Review Guarantee:** Subsequent code PRs are evaluated strictly for code quality, style, and test coverage. Core design, framework choices, and structural layout debates are locked and will not be reopened during code review because they were already settled during the RFC design phase.

### Handling Unexpected Design Blocks

If an insurmountable architectural blocker (such as a critical performance regression or unforeseen security flaw) is uncovered *during* implementation, the Shepherd will pause the PRs. The Shepherd will sync with the Core Architecture Panel to choose a path forward via minor amendment, formal withdrawal, or temporary deferment.

<a id="deprecation-policy"></a>
## Deprecation Policy

Deprecated APIs, configuration keys, and behaviors must emit warnings for **at least 2 minor releases** before removal. This gives downstream users a predictable migration window.

### Process

1. **Deprecate** — Add a runtime warning and a `BREAKING CHANGE:` footer in the commit message. Update docs to mark the item as deprecated.
2. **Mark** — The deprecated item appears in the BREAKING CHANGES section of the next release's changelog. Migration guidance is included in the release notes.
3. **Remove** — No earlier than 2 minor releases after step 1. The removal commit also carries a `BREAKING CHANGE:` footer.

### Per-component examples

**Go** — Add a `// Deprecated:` godoc comment and log a warning on first use:
```go
// Deprecated: Use NewClientV2 instead. Will be removed in v0.5.0.
func NewClient(cfg Config) *Client { ... }
```

**Python** — Use `warnings.warn` with `DeprecationWarning`:
```python
import warnings
warnings.warn(
    "michelangelo.v1.Client is deprecated; use michelangelo.v2.Client instead. "
    "Removal planned for v0.5.0.",
    DeprecationWarning,
    stacklevel=2,
)
```

**Proto** — Mark the field or enum value with `[deprecated = true]` and add a comment:
```protobuf
// Deprecated: use PIPELINE_STATE_RUNNING instead. Removal in v0.5.0.
PIPELINE_STATE_ACTIVE = 1 [deprecated = true];
```

**Helm** — Document the deprecation in `values.yaml` comments and the chart's NOTES.txt:
```yaml
# DEPRECATED: use 'server.resources' instead. Will be removed in v0.5.0.
resources: {}
```

### Reference

- See [UPGRADING.md](./UPGRADING.md) for examples of past migrations.
- See the [Versioning Policy](./docs/getting-started/roadmap.md#versioning-policy) for stability level guarantees.

