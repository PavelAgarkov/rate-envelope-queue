## Versioning & Branching Policy

- Major version branches: Versions are maintained in branches v1, v2, … vn. Changes are developed separately for each major version. Release tags are created from the corresponding version branches to make multi-version maintenance easier.

- main branch: Always tracks the current latest major version to keep the repository easy to navigate for users.

- Older versions & deprecation: Lower major version branches remain alive for stabilization/bug fixes only. A version branch is marked deprecated when there is a branch that is two major versions ahead.

   - Example: If we have version v1, fixes and backward-compatible improvements for v1 are released until a stable v3 is published. After v3 ships, v1 enters passive maintenance. After a stable v4, v1 becomes end-of-life (no longer supported).

## Development Workflow

- Feature branches: Development happens in feature branches.
  - Create your feature branch from pre-release-v1.
  - pre-release-v1 itself is branched from v1.
- Integration flow:
1. Open a PR from your feature branch into pre-release-v1.
2. The feature is tested and refined on pre-release-v1.
3. Once validated, it is merged into v1.
4. A new tag is cut from v1.