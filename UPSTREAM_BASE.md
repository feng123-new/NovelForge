# NovelForge upstream base

NovelForge imported the non-workflow source lineage of `voocel/ainovel-cli` as its engineering base.

- Upstream repository: https://github.com/voocel/ainovel-cli
- Upstream branch: `main`
- Original upstream commit: `c0900290be8dfbae4d1614726e48b53259efbd47`
- Upstream commit date: 2026-08-25
- License: Apache License 2.0

The initial import ran inside GitHub Actions. GitHub would not allow the Actions installation token to push fetched history containing `.github/workflows`, so that path was removed from every imported historical commit before publication. The filtering operation rewrote commit SHAs, retained the non-workflow source trees and the author/date/message metadata of surviving commits, and may have pruned commits whose only changes were workflow files. Parent links around any pruned commits were necessarily reconnected.

The target history therefore preserves auditable source provenance but is not a bit-for-bit mirror of upstream Git object IDs. Use the original upstream SHA above, plus `git range-diff`, `git cherry` and content diffs, when comparing future upstream changes. NovelForge-owned workflows are added in explicit local commits.

Restore the long-lived upstream remote in a new clone with:

```bash
git remote add ainovel-upstream https://github.com/voocel/ainovel-cli.git
git fetch ainovel-upstream --tags
```
