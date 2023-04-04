#!/usr/bin/env bash

# Replaces versions of 'teleport' and 'teleport/api' in go.mod
# with matching pseudo-versions of the given tag or branch.

set -e

which curl >/dev/null || { echo "curl is required" && exit 1; }
which date >/dev/null || { echo "date is required" && exit 1; }
which jq >/dev/null || { echo "jq is required" && exit 1; }
which sed >/dev/null || { echo "sed is required" && exit 1; }

function sed_inline() {
  sed -i'' "$@"
}

version=$1
[ -n "$version" ] || { echo "teleport version (tag like 'v1.2.3' or branch name like 'branch/v123') is required as the only argument to the script" && exit 1; }

ref="heads/$version"
if [[ "$version" = v* ]]; then
  ref="tags/$version"
fi

object_url=$(curl -sS --fail \
  "https://api.github.com/repos/gravitational/teleport/git/ref/$ref" \
  | jq -r .object.url)

object=$(curl -sS --fail "$object_url")
object_date=$(echo "$object" | jq -r .committer.date | sed 's/[-:TZ]//g')
object_sha="$(echo "$object" | jq -r .sha)"
pseudo_version="v0.0.0-${object_date}-${object_sha:0:12}"

sed_inline -e $"s#^\tgithub.com/gravitational/teleport .*#\tgithub.com/gravitational/teleport $pseudo_version // ref: $ref#" go.mod
sed_inline -e $"s#^\tgithub.com/gravitational/teleport/api => .*#\tgithub.com/gravitational/teleport/api => github.com/gravitational/teleport/api $pseudo_version // ref: $ref#" go.mod

go mod tidy
