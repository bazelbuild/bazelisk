#!/bin/bash

# Use the first tag that points to the current HEAD. If no tag is found, the
# summary given by `git describe` is used as a fallback (it contains the most
# recent tag name, the number of commits since, and the short git hash).

CURRENT_TAG=$(git tag -l --points-at HEAD | head -n1)
CURRENT_COMMIT=$(git describe --tags HEAD)

echo "STABLE_VERSION ${CURRENT_TAG:-$CURRENT_COMMIT}"
# rules_nodejs expects to read from volatile-status.txt
echo "BUILD_SCM_VERSION ${CURRENT_TAG:-$CURRENT_COMMIT}"
