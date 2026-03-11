# Releasing

## First release example

To publish `v0.1.0`:

```bash
go test ./...
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Pushing the tag triggers `.github/workflows/release.yml`, which:

1. Runs `go test ./...`
2. Builds release archives with GoReleaser
3. Creates or updates the GitHub Release for the tag
4. Uploads compiled assets and `checksums.txt`

## Release checklist

1. Make sure `main` is green and includes the code you want to ship
2. Run `go test ./...` locally
3. Confirm `git status --short` is clean
4. Create an annotated semver tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
5. Push the tag: `git push origin vX.Y.Z`
6. Watch the Actions run for the `release` workflow
7. Download one asset and verify it with `xero version`

## Expected assets

GoReleaser publishes archives named like these:

- `xero_0.1.0_darwin_amd64.tar.gz`
- `xero_0.1.0_darwin_arm64.tar.gz`
- `xero_0.1.0_linux_amd64.tar.gz`
- `xero_0.1.0_linux_arm64.tar.gz`
- `xero_0.1.0_windows_amd64.zip`
- `xero_0.1.0_windows_arm64.zip`
- `checksums.txt`

## Local dry run

If you have GoReleaser installed locally, you can validate the config without publishing:

```bash
goreleaser release --snapshot --clean
```
