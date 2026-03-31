# My Prompts

Prompts used in this session for open-source documentation and codebase work.

---

## CI / PR Investigation

```
investigate failure in https://github.com/michelangelo-ai/michelangelo/pull/1019
```

```
commit the changes and update PR
```

---

## Python Environment

```
cd python and run "PYTHONPATH=. poetry run python ./examples/bert_cola/bert_cola.py remote-run --image docker.io/library/examples:latest --storage-url s3://default --yes" and investigate the failure
```

---

## PyPI README — Team Setup

```
create a team for open source document project with purpose "prepare open source python package readme in pypi website".
- engineer agent who understand the product from codes: python library and make sure the features are implemented
- tech writer agent who communicate with engineer and product manager to suggest high quality (open source document quality) docs in docs/user-guides/set-up-triggers
- product-manager agent who understand the open source goal and plan for the useful product information consistently in the document docs.
Please review for documents under python/README.md that is for pypi package website in package index page
```

```
talk to tech writer to update python/README.md
```

```
talk to product manager and tech write to make the connection for the doc site - https://michelangelo-ai.github.io/michelangelo/docs for the full user guide and github site - https://github.com/michelangelo-ai/michelangelo
```

```
talk to tech writer to verify the examples again
```

---

## Getting Started Docs — Team Setup

```
create a team for open source document project with purpose "prepare open source user document for the open source document quality".
- engineer agent who understand the product from codes: api, controller manager, UI and python library and make sure the features are implemented
- tech writer agent who communicate with engineer and product manager to suggest high quality (open source document quality) docs under docs/getting-started/
- product-manager agent who understand the open source goal and plan for the useful product information consistently in the document docs.
Please review for documents under docs/getting-started/ for the getting started user guide who reference for the simple start
```

```
talk to product manager that we have operator guide who will adopt the platform integrate with their system in docs/operator-guides/ and contributor guide who to contribute to the code in docs/category/contributing for the details
```

---

## Doc Cleanup & Decisions

```
talk to tech writer to delete docs/getting-started/setup-ide-and-bazel.md
```

```
how about docs/getting-started/custom-docker-images-for-feature-branches-and-sandbox-testing.md location, does this fit in getting-started? ask to product manager and tech writer to discuss
```

```
review the broken links
```

```
fix an error: × Module build failed (from ./node_modules/@docusaurus/mdx-loader/lib/index.js) [Markdown link with URL ../getting-started/setup-ide-and-bazel.md couldn't be resolved]
```

```
question to tech writer and product manager why python ide in getting started when go ide in contributor?
```

---

## Git & PR

```
commit the changes and create PR
```

---

## Memory

```
save all prompts
```

```
save it to md file
```

```
create md file for my prompts
```
