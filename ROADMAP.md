# Deploya Roadmap — becoming the go-to CI & release generator

The goal: a developer runs one command in any repo and gets a CI pipeline and a
release flow that work on the first push, with zero YAML knowledge and zero
secrets to configure for the default path.

## v0.5 — Correctness (done in this pass)

- `deploya release` accepts `GITHUB_TOKEN` as well as `GH_TOKEN`; generated
  release pipeline uses the built-in token so releases need no PAT setup.
- `.releaserc` is parsed with a real YAML parser (inline comments no longer
  break it; `archive` is no longer dropped on save).
- Latest git tag is the source of truth for versioning; `.releaserc` drift is
  detected and reported.
- Changelog compare links use real tag names on both sides.
- Generated CI: explicit `permissions` (GHCR pushes now work with the default
  token), `concurrency` cancellation, images are built-but-not-pushed on PRs,
  `pytest` is installed explicitly, `:latest` tag on main.
- Node projects: yarn / pnpm / missing-lockfile detection with matching
  install, cache, build, and test commands.
- `deploya add`: no duplicate jobs, no dangling `needs:` references, correct
  `workflow_run` names for deploy workflows, `stale` is a scheduled workflow,
  CodeQL language auto-detected, Trivy pinned to a version.
- `deploya preview` and `deploya validate` implemented; `--version` added.
- First unit tests: template output validated as YAML across all 180
  language × registry × notify combinations.

## v0.6 — Trust the output

- **actionlint integration** in this repo's CI: generate pipelines for a matrix
  of fixture projects and run them through actionlint, not just a YAML parser.
- **`deploya validate` upgrade**: check `uses:` action versions exist, warn on
  unpinned actions, expression syntax check for `${{ }}` blocks.
- **Idempotent `init`**: re-running must never clobber a hand-edited ci.yml
  without asking; add `--force` and a diff preview before overwrite.
- **`deploya update`**: bump action versions in an existing generated pipeline.
- **Monorepo awareness**: detect multiple languages, generate a job per
  sub-project with `paths:` filters instead of silently picking the first match.

## v0.7 — Releases that deliver artifacts

- **Implement `archive: true`** — it is currently accepted but does nothing.
  Build a matrix (linux/darwin/windows × amd64/arm64) for Go projects and
  upload archives via the existing `UploadAsset` (currently dead code), or
  shell out to goreleaser when present.
- **`bump: none` path**: docs/chore-only pushes should be able to skip a
  release (configurable `release_on: [feat, fix]`).
- **Ship deploya itself as binaries**: Homebrew tap, `go install`, and a
  curl-able install script; stamp `cmd.Version` via ldflags in release CI.
- **Rename `.releaserc`** (or support an alternative like `deploya.yml`) — the
  name collides with semantic-release's config file and confuses tooling.

## v0.8 — Wider reach

- More languages: PHP (composer), .NET, Elixir; runtime version detection from
  `mise`/`asdf` tool-versions files.
- More CI targets: GitLab CI and, later, Bitbucket — behind a `--target` flag
  with the same detector front-end.
- Registry: implement the `registry` field in `.releaserc` (currently unused);
  push a tagged image on release, not just on CI sha.
- Non-interactive mode (`--yes` + flags for every prompt) so deploya itself is
  scriptable in CI.

## v1.0 — Polish

- README/docs site rewrite with per-language examples and a "generated file
  tour"; remove stale sample output.
- `deploya doctor`: check repo settings (branch protection vs release
  commits, Actions permissions, missing secrets) via the GitHub API.
- Telemetry-free usage feedback: `deploya init` ends with a link to a
  1-question feedback form.

## Engineering hygiene (continuous)

- Keep the template YAML-validation test green for every new option.
- Add golangci-lint to this repo's CI (the generated `lint` job is still a
  placeholder — dogfood a real one here first).
- Error messages must always say what to do next, not just what failed.
