# CI Tooling Upgrade Design

## Goal
Create one PR from the newest `main` commit that upgrades all pinned CI/build tooling versions used by this repository, while preserving current workflow behavior.

## Scope
- In scope:
  - GitHub Actions versions in workflow and local composite-action files.
  - Build tooling versions pinned in Gradle config (wrapper, static-analysis tools, build plugins used for CI/build).
  - Minimal compatibility edits required only when a version bump would otherwise break CI.
- Out of scope:
  - Minecraft/Fabric/runtime dependency upgrades.
  - Feature or behavior changes unrelated to tooling updates.

## Constraints
- Keep PR focused on version/tooling updates only.
- Keep existing workflow triggers, job boundaries, permissions, and artifact behavior unchanged unless required by upgraded tooling.
- Prefer smallest possible diff per file.
- Use stable releases (not snapshots/RCs) unless a tool is snapshot-only in current project usage.

## Current Touchpoints
- `.github/workflows/gradle.yml`
- `.github/actions/gradle/action.yml`
- `gradle/wrapper/gradle-wrapper.properties`
- `build.gradle`

## Approach
1. Sync to newest `main` commit.
2. Inventory every pinned tooling version in touchpoints.
3. Resolve latest stable versions for each pinned item.
4. Update only outdated pins.
5. Apply only required compatibility edits for any major-version behavior change.
6. Validate with local CI-equivalent commands.
7. Open a single PR with concise per-item bump notes.

## Version-Resolution Rules
- GitHub Actions:
  - Bump `uses:` references to latest major/tag accepted by upstream action docs.
  - Keep same action family (no provider swap) unless upstream deprecates/removes the current action.
- Gradle wrapper:
  - Bump to latest stable `gradle-<x.y>-bin.zip`.
- Gradle tooling:
  - Bump explicitly pinned versions for static analysis/build tooling configured in `build.gradle` (e.g., PMD, Checkstyle, plugin versions used in CI path).
  - Do not bump gameplay/runtime dependency coordinates.

## Compatibility Policy
- If a bump breaks existing tasks, add the minimum config change that restores prior intent.
- No speculative refactors or architecture changes.

## Validation Plan
- `./gradlew --version` (wrapper resolves and runs)
- `./gradlew checkstyleMain`
- `./gradlew pmdMain`
- `./gradlew build`
- Optional sanity check: YAML lint/parse via GitHub workflow parser expectation (no syntax regressions)

## PR Deliverable
- Single PR titled as tooling/CI upgrade.
- Diff contains:
  - Version bumps.
  - Strictly necessary compatibility edits.
- PR body includes table/list: old version → new version for each updated item.

## Success Criteria
- All targeted tooling pins are current stable versions.
- Local CI-equivalent checks pass after updates.
- No runtime dependency upgrades included.
- Workflow behavior remains materially unchanged.

