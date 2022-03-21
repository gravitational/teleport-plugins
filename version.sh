#!/bin/sh
#
# Copyright 2022 Gravitational, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# this versioning algo:
#  - if on a tagged commit, use the tag with any preceding 'v' stripped
#    e.g. 6.2.18 (for the commit tagged v6.2.18)
#  - if last tag was a regular release, bump the minor version, make a it a 'dev' pre-release, and append # of commits since tag
#    e.g. 5.5.38-dev.5 (for 5 commits after v5.5.37)
#  - if last tag was a pre-release tag (e.g. alpha, beta, rc), append number of commits since the tag
#    e.g. 7.0.0-alpha.1.5 (for 5 commits after v7.0.0-alpha.1)


increment_patch() {
    # increment_patch returns x.y.(z+1) given valid x.y.z semver.
    # If we need to robustly handle this, it is probably worth
    # looking at https://github.com/davidaurelio/shell-semver/
    # or moving this logic to a 'real' programming language -- 2020-03 walt
    major=$(echo $1 | cut -d'.' -f1)
    minor=$(echo $1 | cut -d'.' -f2)
    patch=$(echo $1 | cut -d'.' -f3)
    patch=$((patch + 1))
    echo "${major}.${minor}.${patch}"
}


SHORT_TAG=`git describe --abbrev=0 --tags --match "v*"`
LONG_TAG=`git describe --tags --match "v*"`
COMMIT_WITH_LAST_TAG=`git show-ref --tags --dereference ${SHORT_TAG}`
COMMITS_SINCE_LAST_TAG=`git rev-list  ${COMMIT_WITH_LAST_TAG}..HEAD --count`
BUILD_METADATA=`git rev-parse --short=8 HEAD`
DIRTY_AFFIX=$(git diff --quiet || echo '-dirty')

# strip leading v from git tag, see:
#   https://github.com/golang/go/issues/32945
#   https://semver.org/#is-v123-a-semantic-version
if echo "$SHORT_TAG" | grep -Eq '^v'; then
    SEMVER_TAG=$(echo "$SHORT_TAG" | cut -c2-)
else
    SEMVER_TAG="${SHORT_TAG}"
fi

if [ -z "$SEMVER_TAG" ]; then # no git tags found, cannot determine version
    exit 1
fi

if [ "$LONG_TAG" = "$SHORT_TAG" ] ; then  # the current commit is tagged as a release
    echo "${SEMVER_TAG}${DIRTY_AFFIX}"
elif echo "$SHORT_TAG" | grep -Eq ".*-.*"; then  # the current ref is a descendant of a pre-release version (e.g. rc, alpha, or beta)
    echo "$SEMVER_TAG.${COMMITS_SINCE_LAST_TAG}+${BUILD_METADATA}${DIRTY_AFFIX}"
else   # the current ref is a descendant of a regular version
    SEMVER_TAG=$(increment_patch ${SEMVER_TAG})
    echo "$SEMVER_TAG-dev.${COMMITS_SINCE_LAST_TAG}+${BUILD_METADATA}${DIRTY_AFFIX}"
fi
