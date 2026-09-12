# Public launch and release checklist

The first planned release is `0.1.0`. Keep the repository private and do not
publish a tag, GitHub release, or npm package until the maintainer approves
publication. Completing local checks does not grant that approval.

## Prepare while private

1. Run `make check`, `make vuln`, and `make verify-gen` on the exact commit to
   release. Review all platform CI jobs. Resolve runner billing or scheduling
   failures separately from code failures.
2. Run `goreleaser check` and `goreleaser release --snapshot --clean --skip=sign` using
   GoReleaser v2.18.1. Inspect all six archives (macOS, Linux, Windows; amd64 and
   arm64) for the binary and license. Run native binaries with `version` and
   `--help`. A cross-build alone does not prove the binary runs on that target.
3. Installer tests run in `make check`. Run `npm test --prefix npm`,
   `node --test scripts/check-release.test.mjs`, and
   `node scripts/check-artifacts.mjs` after the snapshot build. The snapshot
   does not sign or publish; signing is verified in the approved release workflow.
   Run `npm pack --dry-run` in `npm/` and confirm the package includes its license.
4. Run the [live integration test](../CONTRIBUTING.md#the-live-integration-test)
   against the candidate binary with a token supplied by a secret manager.
   Record the commit, platform, and result, without credentials or private
   design data. Require an executed pass, not a skipped job. Also follow the
   [walkthrough](walkthrough.md) through authentication, discovery, context,
   and rendering. Missing credentials mean live behavior remains unverified.
5. Confirm control of the npm name `figctl` and the intended publishing account.
   Package-name availability is not proof of ownership. Review
   [npm trusted publishing](https://docs.npmjs.com/trusted-publishers/) before
   setting up the publisher. Keep public registry publication for the approved
   launch stage.

## Activate after approval

1. Obtain explicit maintainer approval to make the repository public and
   publish the first release. Change visibility only after that approval.
2. Verify private vulnerability reporting using the link in `SECURITY.md`,
   secret scanning, push protection, and branch protection in GitHub. Record
   unavailable settings as unverified, and resolve the reporting path before
   announcing the project. Test external access with a separate account.
3. Move `Unreleased` entries into a dated `## [0.1.0] - YYYY-MM-DD` heading,
   start a new `Unreleased` section, and keep `npm/package.json` at `0.1.0`.
   Remove the pending-release notices from the README and contributing guide.
   Commit and validate the release state with
   `node scripts/check-release.mjs v0.1.0`.
4. Set the repository variable `PUBLIC_RELEASE_ENABLED` to `true`. Both
   publication workflows require this opt-in and a public repository. Tag the
   validated `main` commit as `v0.1.0` and push that tag. Watch the release
   workflow through completion; approve the protected `release` environment
   as the repository owner when prompted; do not infer publication from a successful
   build.
5. Download the release assets without authentication. Verify the signature on
   `checksums.txt` against the exact release workflow and tag identity shown in
   the [installation instructions](../README.md#installation), then verify the
   archive checksums. Confirm every supported OS/CPU archive exists.
6. For the first npm publication, follow npm's current account setup and
   bootstrap requirements, and publish only the matching `0.1.0` package after
   release assets exist. Configure the trusted publisher for this repository's
   `npm-publish.yml` workflow and its protected `release` environment. For subsequent publication, dispatch that workflow
   with the matching tag. It checks the package version and release assets
   before publishing. Never put an npm token in source or command arguments.
7. In clean environments without GitHub credentials, verify the macOS/Linux
   installer with both latest and `FIGCTL_VERSION=v0.1.0`; verify npm global,
   project-local, and `npx figctl@0.1.0 version` on macOS/Linux/Windows; run
   `go install github.com/tiaanduplessis/figctl/cmd/figctl@v0.1.0`; and run the
   downloaded archives on supported targets. Every route must report `0.1.0`.
8. Re-run the walkthrough using an installed release, then announce the release
   with links to the quickstart, walkthrough, and support page.

## If publication only partly succeeds

Stop promotion and retain workflow logs without secrets. Check which tag,
GitHub assets, signatures, and npm version actually exist before retrying.
If GitHub assets are incomplete, do not publish npm. If GitHub succeeded but
npm failed, resolve npm setup and retry that stage against the same verified
assets. Do not overwrite published binaries or reuse an npm version for changed
content; ship a patch version. Explain affected installation routes in the
release notes. Changing public visibility back to private cannot retract copies
already downloaded.
