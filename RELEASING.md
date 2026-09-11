# Releasing Nocturne

GitHub Actions builds every release on native hosted runners. A stable SemVer tag such as `v1.0.0` produces:

- Windows x64 portable ZIP and per-user NSIS installer;
- macOS Apple Silicon and Intel portable app ZIPs and DMG images;
- Linux x64 portable tarball, DEB, and RPM packages;
- `SHA256SUMS.txt` for all downloadable files.

Create a release by pushing a tag:

```shell
git tag -a v1.0.0 -m "Nocturne 1.0.0"
git push origin v1.0.0
```

The workflow can also be started manually from **Actions → Release** with a stable version number. It creates the matching tag at the selected commit.

## Optional code signing

Unsigned builds are still produced, which is useful for a university project and reproducible testing. Public distribution should configure these GitHub Actions secrets:

- `WINDOWS_CERTIFICATE_BASE64` and `WINDOWS_CERTIFICATE_PASSWORD` for an Authenticode PFX certificate;
- `APPLE_CERTIFICATE_BASE64`, `APPLE_CERTIFICATE_PASSWORD`, and `APPLE_SIGNING_IDENTITY` for a Developer ID Application certificate;
- `APPLE_ID`, `APPLE_TEAM_ID`, and `APPLE_APP_PASSWORD` to notarize and staple macOS DMGs.

The release workflow signs when the corresponding secrets are present. Keep signing keys only in GitHub Actions secrets.
