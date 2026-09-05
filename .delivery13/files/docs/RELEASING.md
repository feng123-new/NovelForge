# Candidate publication and stable acceptance

Phase 13A delivers an installable **release candidate** and its verification tools. Phase 13B remains explicit local acceptance. Building six targets is not testing six operating-system environments; see the manifest for native smoke results and unexecuted scopes.

## Three execution channels

`quick.yml` runs named, bounded checks on PRs and main. It uses the same `scripts/verify.py --mode quick` as local development. Source/contract/entry/frontend failures fail the job. It does not use continue-on-error to manufacture green checks.

The previous comprehensive `ci.yml` is retained as **Full acceptance (manual)** with its jobs and assertions intact; only its trigger is changed to workflow_dispatch. Windows, Docker, full Go/frontend and race remain available there and through the local manual plan. Existing GoReleaser full pre-hooks are also retained for a later explicitly authorized stable release; candidate packaging uses a separate script and does not silently bypass a stable gate.

`candidate.yml` is explicitly triggered from the Actions page with a new `vMAJOR.MINOR.PATCH-rc.N` or by creating a `release/vMAJOR.MINOR.PATCH-rc.N` branch at a merged main commit. It rejects non-merged sources and existing release/tag identities. It runs quick checks, packages the exact clean commit, uploads all assets to a draft, verifies names/sizes/digests, and publishes only as a prerelease with latest=false. Build jobs have read-only repository permissions; only the final publication job receives contents:write. No user model secrets are supplied.

## Artifacts and evidence

Six archives cover Linux/Darwin/Windows amd64 and arm64. File naming matches the existing explicit-version installer. Each archive includes the binary, BUILD_INFO.json, example configuration, start helpers, local smoke script, documentation and third-party notices. Browser files are compiled from committed web/dist and checked for exact reproduction first. Archives contain no novel workspace or actual model configuration.

`release-manifest.json` records version, source commit/tree, toolchain, frontend hashes, binary/archive hashes and native-vs-cross-compiled status. `verification-summary.json` records the executed quick checks and unexecuted scopes; full test logs stay in the workflow artifact. `novelforge_checksums.txt` covers archives and both manifests. These are checksum protections, not OS code signing, Apple notarization or a provider-independent supply-chain attestation claim.

A packaging failure stops before publication. A publication failure can leave an unpublished draft; inspect it before manual cleanup/retry. Never force-update a published tag or silently replace downloaded assets. Create a new rc number for subsequent source changes. After successful publication, archive and remove the exact temporary release-control branch; keep the immutable tag and release.

## Local packaging (does not publish)

Use a clean checkout, Go as specified by go.mod, Node 22 and Python 3.10+:

```sh
python scripts/verify.py --mode quick --output verification-results/release
python scripts/package_candidate.py --version v0.1.0-rc.2 --verification verification-results/release/result.json --output candidate-dist
```

The output directory must not exist. The report must match HEAD and tree exactly, have no tracked source modifications, and include every required candidate check. Editing frontend source requires regenerating and committing exact assets before a clean candidate build. No full-suite or paid-model claim is inferred from a passing quick report.

## Stable release decision

Use [LOCAL_ACCEPTANCE.md](LOCAL_ACCEPTANCE.md) to record complete tests on the target machine, recovery and actual provider behavior. Attach evidence with exact source identity, not only screenshots of green CI. Resolve blocking defects, rerun affected/full required checks, and publish a new candidate as needed. Only after Phase 13B approval should a stable tag/latest release be created using the retained full release gates. This candidate workflow refuses stable-version tags by design.
