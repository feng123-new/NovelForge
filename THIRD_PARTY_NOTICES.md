# Third-Party Notices

NovelForge is distributed under the Apache License 2.0. This file records code provenance and design references so that license boundaries remain explicit during ongoing development.

## voocel/ainovel-cli

- Repository: https://github.com/voocel/ainovel-cli
- License: Apache License 2.0
- Original imported commit: `c0900290be8dfbae4d1614726e48b53259efbd47` (2026-08-25)
- Use in NovelForge: direct code base and continuing upstream

NovelForge preserves the upstream source history, authorship, copyright notices, and Apache-2.0 license. During the initial GitHub-hosted import, `.github/workflows` was filtered from imported historical commits because the GitHub Actions installation token was not permitted to introduce workflow files from fetched history. The source graph, authors, dates, messages, and all non-workflow code were retained; exact import details are recorded in `UPSTREAM_BASE.md`.

Files directly derived from or modified from ainovel-cli remain subject to the Apache License 2.0 requirements, including retention of notices and prominent identification of changes where applicable.

## Nigh/show-me-the-story

- Repository: https://github.com/Nigh/show-me-the-story
- License: MIT
- Copyright: Copyright (c) 2026 xianii
- Use in this phase: product/UI/architecture research only

No show-me-the-story source code is incorporated in the current NovelForge phase. If MIT-licensed code is incorporated in a future commit, the relevant copyright and complete MIT license text must be retained with the copied or modified files and this notice must be updated before merge.

## Hurricane0698/novelwriter

- Repository: https://github.com/Hurricane0698/novelwriter
- License: GNU Affero General Public License v3.0
- Use in NovelForge: public documentation, observable UI behavior, and architecture concepts only

No source code from novelwriter is copied into NovelForge. Structured World Model, Entity/Relation/System, Studio/Atlas, and proposal-review-authoritative-fact concepts are reimplemented clean-room under NovelForge's own design and code.

## EthanYoQ/AI-Novel-Writer

- Repository: https://github.com/EthanYoQ/AI-Novel-Writer
- License: GNU General Public License v3.0
- Use in NovelForge: public workflow concepts only

No source code from AI-Novel-Writer is copied into NovelForge. Premise/Character/World/Outline/Blueprint/Draft/Review/Revision/Final, chapter version, proposal, and diff concepts are reimplemented clean-room.

## Go dependencies

Go module dependencies retain their own licenses. Release and source distributions must preserve license information required by those dependencies. A machine-generated dependency license inventory will be added to the release pipeline before v0.1.0.
