#!/bin/bash -e

TAG=$(gh release view --json name | jq -r .name)
checksums=$(gh release download --pattern '*_checksums.txt' -O -)
echo "Updating krew manifest for commatrix to $TAG"

# Create a new branch with commatrix_version name
BRANCH="commatrix_$TAG"
git checkout -b "$BRANCH"

# Update version in commatrix-krew.yaml
echo "Updating version to $TAG"
VERSION="$TAG" yq -i '.spec.version = strenv(VERSION)' cmd/commatrix-krew.yaml

# Update artifact hashes
while read -r checksum filename; do
    uri="https://github.com/openshift-kni/commatrix/releases/download/$TAG/$filename"
    echo "Updating artifact $uri ($checksum)"
    FILENAME="$filename" URI="$uri" SHA="$checksum" yq -i '(.spec.platforms[] | select(.uri|contains(strenv(FILENAME)))) |= . + {"uri": strenv(URI), "sha256": strenv(SHA)}' cmd/commatrix-krew.yaml
done <<<"$checksums"

# Commit and push changes
git add cmd/commatrix-krew.yaml
git commit -am "Version bump commatrix to $TAG"
git push origin "$BRANCH"

# Create a PR
gh pr create --fill --web