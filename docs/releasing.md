# Verified release process

HAMi-WebUI has one stable release controller: `.github/workflows/release.yaml`.
It provides two recurring release phases plus a one-time OCI bootstrap phase,
and serializes every run through the `stable-release` concurrency group. The
legacy tag-triggered chart workflow is removed.

This workflow is deliberately fail-closed. Merging it does not enable stable
publishing: the protected `release` environment must exist and its
`STABLE_RELEASES_ENABLED` variable must be set to `true` after the repository,
registry, and release acceptance gates below are configured.

## What “atomic” means here

Docker Hub, GHCR, GitHub Pages, and GitHub Releases do not share a transaction.
The controller therefore makes an ordered, resumable release:

1. build each multi-platform image once and address it by digest;
2. package the chart once and preserve that exact archive;
3. test that archive against the release issue acceptance matrix while the
   `release` environment is waiting;
4. preflight every destination, stage the private GitHub draft and assets, and
   reserve the exact release tag before any registry or Pages stable write;
5. promote manifests and publish the same archive to every chart endpoint;
6. publish the GitHub Release as the final mutation and public completion
   signal.

Every publication step follows the same retry rule:

- absent: create it;
- present and byte/digest/commit identical: do nothing;
- present but different: stop; never overwrite it.

If publication is interrupted, use GitHub's **re-run failed jobs** on the same
workflow run so `prepare-release` is not rerun and the canonical artifact is
reused. A brand-new dispatch may package different archive bytes and is safe
only before any stable destination was written. If the canonical artifact is
lost after a partial publication, recover those exact bytes from the staged
destination or cut a new patch version. A bad published version always receives
a new patch version; tags, images, packages, assets, and index entries are never
replaced.

## Release identity contract

For stable version `X.Y.Z`:

| Object | Required identity |
| --- | --- |
| Git tag and GitHub Release | `vX.Y.Z` |
| `Chart.yaml` `version` | `X.Y.Z` |
| `Chart.yaml` `appVersion` | `X.Y.Z` |
| both default image tags | `vX.Y.Z` |
| both default image digests | candidate Docker Hub manifest digests |
| chart package | `hami-webui-X.Y.Z.tgz` |

Only `vX.Y.Z` is created. Do not create a second `hami-webui-X.Y.Z` tag or
release. The chart renders `repository@digest` when a digest is present; the
stable tag remains a human-readable alias and must resolve to that same digest.
Docker Hub and GHCR digests are recorded separately and are not assumed to be
equal.

## Phase 1: candidate images

Run **Verified Stable Release** with:

- `phase`: `candidate`
- `source_sha`: the full, current `main` commit SHA

The workflow refuses a branch, abbreviated SHA, stale `main`, or fork. It builds
the frontend and backend once for `linux/amd64` and `linux/arm64`, pushing only a
unique candidate tag:

```text
candidate-<source-sha-prefix>-<run-id>-<attempt>
```

Both Docker Hub and GHCR manifest digests are sealed into the
`release-candidate-<run-id>` artifact. Keep that run ID.

Create a small release-metadata PR that sets the next Chart version,
`appVersion`, default tags, and the two Docker Hub digests from the candidate.
After the candidate source, only these paths may change:

- `charts/hami-webui/Chart.yaml`
- `charts/hami-webui/values.yaml`
- `charts/hami-webui/README.md`
- `CHANGELOG.md`
- `docs/releases/**`

Any application, build, dependency, workflow, or template change invalidates
the candidate; build a new one instead. Within `Chart.yaml`, only `version` and
`appVersion` may differ. Within `values.yaml`, only the frontend/backend `tag`
and `digest` fields may differ; the controller compares the remaining YAML
structure to the candidate commit.

## Phase 2: canonical chart and protected publication

After the release-metadata PR is merged, run **Verified Stable Release** on
`main` with:

- `phase`: `publish`
- `candidate_run_id`: the successful phase-1 run ID

The unprotected preparation job checks the originating workflow run and all
release identities. It runs `helm dependency build` from `Chart.lock` with no
dependency update, verifies the lock did not change, records the downloaded
dependency archive hashes, lints/renders the chart, and calls `helm package`
exactly once.

The resulting `release-bundle-<run-id>` contains:

- the canonical `hami-webui-X.Y.Z.tgz`;
- `release-manifest.json`, including source, image, lock, dependency, and chart
  hashes;
- `SHA256SUMS`.

The next job waits for approval on the protected `release` environment. Do not
approve it yet.

## Acceptance gate

Track every stable release in a dedicated release issue. Define the acceptance
environment and risk-based test matrix there, and attach the run-specific
evidence and approval to that issue or to retained workflow artifacts.

Download `release-bundle-<run-id>` from the waiting publish run and verify
`SHA256SUMS`. Test that exact `.tgz`; do not repackage it locally. Keep the
protected `release` job waiting and `STABLE_RELEASES_ENABLED=false` throughout
acceptance.

The acceptance matrix must cover installation and lifecycle safety on a
representative supported GPU/Kubernetes environment, plus device discovery,
allocation, workload metrics, and ordinary WebUI use. An authorized maintainer
must inspect the running WebUI for this exact bundle and explicitly record
approval. Automated checks, silence, or approval of another run do not count.

Only after that approval may `STABLE_RELEASES_ENABLED` be changed to `true` and
the pending protected deployment be approved. Keep the acceptance environment
available until the published artifacts and released WebUI pass post-release
verification.

## Ordered publication

After approval the controller:

1. re-verifies the canonical bundle hashes and performs a read-only preflight
   of all four image tags, the public and repository-linked OCI package/version,
   workflow-built Pages site/package/index, Git tag, GitHub Release, and Release
   assets; every stable object must be absent or an exact match;
2. creates or reuses a private GitHub draft targeting the verified commit and
   attaches the same archive, checksum, and manifest;
3. create-only reserves `vX.Y.Z` at the verified commit; a retry accepts that
   tag only when it already resolves to the same commit;
4. adds `vX.Y.Z` to the exact candidate manifests in Docker Hub and GHCR without
   rebuilding;
5. pushes the same chart archive to
   `oci://ghcr.io/project-hami/charts/hami-webui`, pulls it back both
   authenticated and anonymously, and compares archive SHA256;
6. commits the same archive and its new `index.yaml` entry together on
   `gh-pages`, explicitly deploys that tree with GitHub Actions Pages, then
   verifies the public Pages URL and index digest;
7. re-resolves the tag SHA and rechecks the exact draft asset set and bytes,
   then publishes the already-complete draft as the final mutation;
8. verifies the published Release is immutable and performs only read-only
   checks after publication.

Step 3 is an intentional irreversible checkpoint. If any later destination
fails, resume the same workflow run with the same bundle; never delete, move,
or reuse the reserved tag for different bytes or a different commit.

The OCI manifest digest and chart archive SHA256 identify different objects and
are not expected to be equal. Pull-back archive equality is the cross-endpoint
test.

## Required repository and registry setup

Before setting `STABLE_RELEASES_ENABLED=true`:

- observe the `pr-release-required` check on this controller PR, add that exact
  aggregate context to `main` protection, and require it before merging future
  controller changes;
- create a protected GitHub environment named `release`;
- disable administrator bypass on that environment so maintainers cannot skip
  the recorded acceptance approval;
- deliberately keep the Pages deployment in that same `release` environment
  (rather than using `github-pages`) so one protected approval gates every
  stable write; the existing `github-pages` environment is not the release
  approval gate;
- configure required reviewer(s) and restrict deployments to the intended
  default-branch workflow; while there is only one reliably active maintainer,
  allow that maintainer to approve after attaching explicit acceptance
  evidence, and
  enable “prevent self-review” only when a second release approver is reliably
  available;
- keep environment variable `STABLE_RELEASES_ENABLED=false` until the exact
  canonical bundle has passed the release issue acceptance matrix, then set it
  to `true` immediately before approving the stable publish job;
- retain repository secrets, or selected organization secrets,
  `DOCKERHUB_TOKEN` (username) and `DOCKERHUB_PASSWD` (access token/password);
- allow the workflow's `GITHUB_TOKEN` `contents:write`, `packages:write`, and
  `actions:read` only in the jobs that declare them;
- keep Docker Hub and GHCR image packages public;
- run this workflow once with `phase=bootstrap-oci`. Its protected job publishes
  one `0.0.0-bootstrap.<run-id>` package from a retained artifact and then
  intentionally requires an anonymous pull. A first run may stop because GitHub
  creates the package as private; make `charts/hami-webui` public, confirm it is
  linked to this repository, and use **re-run failed jobs** so the identical
  artifact is verified rather than repackaged;
- change the repository's Pages build source from legacy branch deployment to
  **GitHub Actions**; the controller still commits source artifacts to
  `gh-pages`, then explicitly deploys them because pushes made with
  `GITHUB_TOKEN` do not trigger legacy branch builds;
- serve the deployed site at `https://project-hami.github.io/HAMi-WebUI`;
- block update and deletion of both `v*` and legacy `hami-webui-*` tags with no
  bypass, and block creation of legacy `hami-webui-*` tags. Leave new `v*` tag
  creation under repository write permission: this repository cannot select the
  built-in GitHub Actions App as a ruleset bypass actor. The protected controller
  uses a strict SemVer name, performs a create-only API call, and rejects any
  existing tag that does not already resolve to the verified commit;
- enable immutable releases. Immediately before approving a stable publish, an
  administrator must verify `GET /repos/Project-HAMi/HAMi-WebUI/immutable-releases`
  returns `enabled: true`, or enforce immutable releases for this repository at
  organization level. The workflow's ordinary `GITHUB_TOKEN` intentionally has
  no repository Administration permission, so it cannot perform this settings
  check itself; it does assert the resulting published Release is immutable.

The sealed candidate manifest, recorded digests, and retained workflow artifact
are the release evidence. Candidate tags are unique lookup handles, not assumed
to be registry-enforced immutable objects, and must never be overwritten.
Retention cleanup, if added, must be a separate policy and must never run after
a stable-release publication step.
