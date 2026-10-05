#!/bin/sh
# release-notes.sh TAG prints the release notes for TAG: its section of
# CHANGELOG.md and how to install it. Used for the GitLab release and the
# copy on GitHub, so both say the same.
set -eu
tag=${1:?usage: release-notes.sh vX.Y.Z}
home=https://gitlab.com/codebarge/barge

section=$(awk -v head="## $tag" '
  index($0, head) == 1 { found = 1; next }
  found && /^## /     { exit }
  found               { print }
' CHANGELOG.md)
[ -n "$section" ] || { echo "CHANGELOG.md has no section for $tag" >&2; exit 1; }

# Links to files in the repository (DOCUMENTATION.md) must work from a
# release page too: make them absolute, pinned to this tag.
printf '%s\n' "$section" |
  sed "s#](\([A-Za-z0-9_./-]*\.md\))#]($home/-/blob/$tag/\1)#g" |
  sed '/./,$!d'

cat <<NOTES

---

**Install:** \`go install gitlab.com/codebarge/barge/cmd/barge@$tag\`, or download
a binary below and check it against \`checksums.txt\`.

[Documentation]($home/-/blob/$tag/DOCUMENTATION.md) ·
[Report a bug]($home/-/issues/new?issuable_template=Bug%20report) ·
[Request a pilot of Barge for teams]($home/-/issues/new?issuable_template=Pilot%20request)
NOTES
