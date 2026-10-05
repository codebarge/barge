#!/bin/sh
# github-release.sh copies a release to the GitHub mirror: the same tag, the
# same notes (scripts/release-notes.sh) and the binaries from dist/.
#
# The push mirror brings the tag to GitHub within minutes; this waits for it
# rather than letting GitHub create the tag on another commit. Running it
# again updates the notes and uploads only the missing files.
#
#   CI_COMMIT_TAG=v0.3.0 GITHUB_TOKEN=github_pat_… sh scripts/github-release.sh
#
# GITHUB_TOKEN: a fine-grained token for the repository with
# "Contents: Read and write". Needs curl and jq.
set -eu
tag=${CI_COMMIT_TAG:?set CI_COMMIT_TAG, e.g. v0.3.0}
: "${GITHUB_TOKEN:?set GITHUB_TOKEN}"
repo=${GITHUB_REPO:-codebarge/barge}
api=${GITHUB_API:-https://api.github.com}/repos/$repo
uploads=${GITHUB_UPLOADS:-https://uploads.github.com}/repos/$repo
wait_minutes=${GITHUB_WAIT_MINUTES:-20}

gh() {
  curl -fsS -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28" "$@"
}

ls dist/* >/dev/null 2>&1 || { echo "dist/ is empty: build the binaries first" >&2; exit 1; }

# 1. Wait for the push mirror to bring the tag over.
tries=$((wait_minutes * 2))
i=0
until gh -o /dev/null "$api/git/ref/tags/$tag" 2>/dev/null; do
  i=$((i + 1))
  if [ "$i" -gt "$tries" ]; then
    echo "$tag has not reached GitHub in $wait_minutes minutes." >&2
    echo "Check the push mirror (Settings → Repository → Mirroring repositories), then retry this job." >&2
    exit 1
  fi
  echo "Waiting for the mirror to push $tag to GitHub ($i/$tries)…"
  sleep 30
done

# 2. Create the release, or update its notes if it exists.
notes=$(sh scripts/release-notes.sh "$tag")
case "$tag" in *-rc.*) pre=true ;; *) pre=false ;; esac
payload=$(jq -n --arg tag "$tag" --arg name "Barge CLI $tag" --arg body "$notes" --argjson pre "$pre" \
  '{tag_name: $tag, name: $name, body: $body, prerelease: $pre, make_latest: (if $pre then "false" else "true" end)}')

existing=$(gh "$api/releases/tags/$tag" 2>/dev/null || true)
id=$(printf '%s' "$existing" | jq -r '.id // empty' 2>/dev/null || true)
if [ -n "$id" ]; then
  gh -X PATCH "$api/releases/$id" -d "$payload" >/dev/null
  echo "Updated the GitHub release $tag."
else
  id=$(gh -X POST "$api/releases" -d "$payload" | jq -r .id)
  echo "Created the GitHub release $tag."
fi

# 3. Upload the binaries that are not there yet.
have=$(gh "$api/releases/$id/assets?per_page=100" | jq -r '.[].name')
for f in dist/*; do
  name=$(basename "$f")
  if printf '%s\n' "$have" | grep -qxF "$name"; then
    echo "  $name: already there"
    continue
  fi
  curl -fsS -H "Authorization: Bearer $GITHUB_TOKEN" -H "Content-Type: application/octet-stream" \
    --data-binary "@$f" "$uploads/releases/$id/assets?name=$name" >/dev/null
  echo "  $name: uploaded"
done

echo "https://github.com/$repo/releases/tag/$tag"
