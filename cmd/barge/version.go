package main

// releaseVersion is the release this source tree is. Bump it in every
// release together with CHANGELOG.md; the release job in .gitlab-ci.yml
// fails when the tag does not match it.
const releaseVersion = "v0.3.0"

// version can be set at build time (-ldflags "-X main.version=v0.3.0");
// release builds do. Left empty, the version comes from the module (go
// install ...@vX.Y.Z) or from releaseVersion.
var version = ""
