# NovelForge upstream base

NovelForge was imported from `voocel/ainovel-cli` with its commit graph, authors, dates, and messages retained.

- Upstream repository: https://github.com/voocel/ainovel-cli
- Upstream branch: `main`
- Original upstream commit: `c0900290be8dfbae4d1614726e48b53259efbd47`
- Upstream commit date: 2026-08-25
- License: Apache License 2.0

GitHub Actions workflow paths were removed from every imported historical commit because GitHub does not allow an Actions installation token to introduce workflow files from fetched history. This rewrites commit SHAs but preserves source history outside `.github/workflows`, the parent graph, authorship, timestamps, and commit messages. NovelForge-owned workflows are added in explicit local commits.

Restore the long-lived upstream remote in a new clone with:

```bash
git remote add ainovel-upstream https://github.com/voocel/ainovel-cli.git
git fetch ainovel-upstream --tags
```
