# Scramjet corresponding source

`SOURCE.lock` pins the single upstream commit that carries both production npm
packages. The mapping is proved by the two upstream tags, the package versions
inside `packages/core/package.json` and `packages/controller/package.json`, and
the official npm integrity values recorded in the production lockfile.

The upstream commit has no Git submodules. It declares `AGPL-3.0-only` in package
metadata but does not contain a `LICENSE` or `COPYING` file, and the two npm
tarballs omit the license text as well. A distribution must therefore include a
canonical GNU Affero General Public License version 3 text next to the complete
source archive. This omission is not permission to change the license.

Before release, clone the exact commit, verify both tags and package versions,
and package the complete tree. Do not substitute the current default branch or
an npm tarball: the npm packages contain compiled output, not the complete
corresponding source and build inputs.
