# License Boundaries

## Repository license

NovelForge is licensed under Apache License 2.0. The root `LICENSE` file is retained from the ainovel-cli base. Contributions intended for inclusion are accepted under the same license unless explicitly stated otherwise.

## Direct upstream code

`voocel/ainovel-cli` is the direct source-code upstream under Apache-2.0. NovelForge preserves its history and notices. Direct modifications remain Apache-2.0-compatible and should carry a clear changed/derived notice when the file already has or warrants one.

## MIT code policy

`Nigh/show-me-the-story` is MIT-licensed. The first NovelForge server/Web foundation is an original implementation based on requirements and public product behavior; no source from that repository is copied in the current phase.

Before incorporating MIT code in a future change, the author must:

1. identify the exact source file and upstream revision;
2. preserve copyright and the complete MIT permission/disclaimer text;
3. update `THIRD_PARTY_NOTICES.md`;
4. keep the copied code separable enough for provenance review.

## GPL/AGPL clean-room policy

`Hurricane0698/novelwriter` is AGPL-3.0 and `EthanYoQ/AI-Novel-Writer` is GPL-3.0. They are design references only. Source, tests, generated code, prompts uniquely copied from those repositories, and mechanically translated implementations are prohibited from entering NovelForge.

Allowed inputs are public documentation, high-level architecture concepts, and observable product behavior. NovelForge design documents define independent interfaces and tests first; implementation is written without consulting or transcribing copyleft source.

## Review checklist

Every pull request that touches third-party-derived material must answer:

- Is this direct code reuse or design reference?
- What repository, revision and file is the source?
- Is its license compatible with Apache-2.0 distribution?
- Are required notices present?
- Does the change cross the GPL/AGPL clean-room boundary?

When uncertain, do not merge the copied implementation until the boundary is resolved.
