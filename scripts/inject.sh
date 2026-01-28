#!/usr/bin/env bash

set -x

# Create tmp directory for processing
mkdir -p ./tmp

# Copy the base command script (output without .sh for cleaner CLI usage)
cp ./scripts/oc-commatrix.sh ./tmp/oc-commatrix

# Update image if provided
if [ -n "$IMAGE" ]; then
  echo "updating CLI image to $IMAGE"
  sed -i.bak "s|^img=.*|img=\"$IMAGE\"|" ./tmp/oc-commatrix
  rm -f ./tmp/*.bak
fi

# Output to DIST_DIR or tmp
if [ -z "$DIST_DIR" ]; then
  echo "output generated in tmp folder"
else
  echo "output generated in $DIST_DIR folder"
  mkdir -p ./"$DIST_DIR"
  cp -a ./tmp/. ./"$DIST_DIR"
  rm -rf ./tmp
fi

