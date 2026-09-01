# License Boundaries

## Repository license

NovelForge is licensed under Apache License 2.0. The root `LICENSE` file is retained from the ainovel-cli base. Contributions intended for inclusion are accepted under the same license unless explicitly stated otherwise.

## Direct upstream code

`voocel/ainovel-cli` is the direct source-code upstream under Apache-2.0. NovelForge preserves its history and notices. Direct modifications remain Apache-2.0-compatible and should carry a clear changed/derived notice when the file already has or warrants one.

## Phase 2 SQLite dependency

Phase 2 adds `modernc.org/sqlite` as the SQLite driver. Version `v1.57.0` provides the required CGo-free build path and ships permissive BSD-3-Clause, MIT, and SQLite public-domain notices. The exact selected module version and all transitive modules are recorded in `docs/DEPENDENCY_LICENSES.md` after `go mod tidy` resolves the committed graph.

The dependency inventory is generated from the real module graph and module-cache license files:

```sh
GOWORK=off go mod download all
GOWORK=off go run ./scripts/dependency_license_inventory.go > docs/DEPENDENCY_LICENSES.md
GOWORK=off go run ./scripts/dependency_license_inventory.go -check docs/DEPENDENCY_LICENSES.md
```

CI rejects a stale inventory and rejects dependencies detected as GPL, AGPL, LGPL, SSPL, unknown, or requiring manual review. The inventory generator itself uses only the Go standard library so it does not introduce a hidden audit dependency.

### Reviewed dependency edge cases

The scanner recognizes `LICENSE`, `LICENSE.*`, `LICENSE-*`, `COPYING`, `COPYING.*`, and `COPYING-*` grant files. This is required for dual-licensed modules such as `github.com/aymanbagabas/go-udiff@v0.3.1`, which ships separate `LICENSE-BSD` and `LICENSE-MIT` files.

MPL-2.0 is classified before GPL-family references because the MPL text names GPL, LGPL, and AGPL only as possible secondary licenses. `github.com/hashicorp/golang-lru/v2@v2.0.7` is therefore correctly inventoried as MPL-2.0 rather than GPL.

Two exact module-version reviews cover published snapshots whose module-cache root does not expose a classifiable license file:

- `github.com/mattn/go-localereader@v0.0.1` — MIT. The published snapshot predates the repository's MIT `LICENSE` file, while its source-bearing files and the currently MIT-licensed upstream source have identical Git blob IDs.
- `modernc.org/memory@v1.11.0` — BSD-3-Clause. The authoritative Go package record for the exact version reports BSD-3-Clause and identifies `gitlab.com/cznic/memory` as the source repository; the source headers also state that use is governed by the BSD-style license in the upstream `LICENSE` file.

The corresponding overrides:

- are keyed to the complete `module@version` string;
- apply only when cache-file detection returns `UNKNOWN`;
- cannot hide a detected incompatible license;
- do not apply to future or replacement versions;
- are covered by regression tests that reject broader version matching.

Any additional override requires equivalent version-specific evidence, a written review in this file, and a dedicated regression test.

## MIT code policy

`Nigh/show-me-the-story` is MIT-licensed. The NovelForge server, project repository, migrations, API, tests, and Web foundation are original implementations based on requirements and public product behavior; no source from that repository is copied in Phase 2.

Before incorporating MIT code in a future change, the author must:

1. identify the exact source file and upstream revision;
2. preserve copyright and the complete MIT permission/disclaimer text;
3. update `THIRD_PARTY_NOTICES.md`;
4. keep the copied code separable enough for provenance review.

## GPL/AGPL clean-room policy

`Hurricane0698/novelwriter` is AGPL-3.0 and `EthanYoQ/AI-Novel-Writer` is GPL-3.0. They are design references only. Source, tests, generated code, prompts uniquely copied from those repositories, and mechanically translated implementations are prohibited from entering NovelForge.

Allowed inputs are public documentation, high-level architecture concepts, and observable product behavior. NovelForge design documents define independent interfaces and tests first; implementation is written without consulting or transcribing copyleft source.

No code, SQL, component, test, or prompt in the Phase 2 implementation was copied from either clean-room reference repository.

## Review checklist

Every pull request that touches third-party-derived material must answer:

- Is this direct code reuse or design reference?
- What repository, revision and file is the source?
- Is its license compatible with Apache-2.0 distribution?
- Are required notices present?
- Does the change cross the GPL/AGPL clean-room boundary?
- Does `docs/DEPENDENCY_LICENSES.md` exactly match the resolved lockfile?
- Does the CGo-disabled release build still pass?

When uncertain, do not merge the copied implementation until the boundary is resolved.
