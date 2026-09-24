# Contributing

Thanks for helping! Issues and merge requests live on
[GitLab](https://gitlab.com/codebarge/barge); the GitHub repository is a
read-only mirror.

## Issues

Pick a form, it asks for what we need:

- [Bug report](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Bug%20report): the command you ran and its output are the
  most useful thing you can send. Please do not paste output that shows your
  colleagues' names publicly; replace them.
- [Feature request](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Feature%20request)
- [Question](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Question)
- [Pilot request](https://gitlab.com/codebarge/barge/-/issues/new?issuable_template=Pilot%20request) for Barge for teams (confidential)

## Merge requests

The merge request form has a checklist; in short:

- `go vet ./...`, `gofmt -l .` and `go test ./...` must pass.
- No new dependencies: the module uses the Go standard library only.
- Sign off every commit (`git commit -s`). By signing off you certify the
  [Developer Certificate of Origin](https://developercertificate.org/): you
  wrote the change or have the right to submit it under the Apache 2.0 licence.
