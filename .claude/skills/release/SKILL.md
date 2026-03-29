---
name: release
description: Create a new keytun release — bump version, update changelog, tag, push, and create GitHub release
disable-model-invocation: true
argument-hint: "[version, e.g. v0.9.0]"
---

# Create release $ARGUMENTS

Create a new keytun release with version **$ARGUMENTS**.

## Steps

1. **Validate version**: Ensure `$ARGUMENTS` matches `vX.Y.Z` format and is greater than the latest git tag.

2. **Gather changes**: Run `git log <latest-tag>..HEAD --oneline` to collect all commits since the last release.

3. **Draft release notes**: Categorize commits into user-facing changes. Exclude website-only changes (feat(website), docs(website)) from the release notes — those are not relevant to end users. Group into:
   - Fixes & Improvements (bug fixes, reliability)
   - New Features (if any)
   - Other (test coverage, docs, etc. — only if notable)

4. **Update changelog**: Add a new entry at the top of `website/src/pages/changelog.astro`, following the existing format with `release`, `release-header`, tags (`tag-feat`, `tag-fix`, `tag-security`), etc. Do NOT include website-only changes in the changelog either.

5. **Commit the changelog**: Stage and commit with message `docs(website): add $ARGUMENTS to changelog`.

6. **Create git tag**: `git tag $ARGUMENTS`

7. **Push**: `git push origin main --tags`

8. **Create GitHub release**: Use `gh release create $ARGUMENTS` with the drafted release notes.

9. **Report**: Show the release URL when done.
