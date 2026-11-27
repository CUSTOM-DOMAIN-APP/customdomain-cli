# Changelog

All notable changes to the CustomDomain™ command line are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.2] - 2026-11-13

### Changed
- `customdomain init` asks which application to use when it runs in a terminal and no `--app` is given.

## [0.1.1] - 2026-11-12

### Changed
- The npm package is published as `@customdomain/cli`. Publishing skips versions that are already on npm.

## [0.1.0] - 2026-11-12

### Added
- First release. Commands: `init`, `login`, `logout`, `whoami`, `domains`, `doctor <domain>` and `version`.
- `init` signs you in through the browser, creates an API key for your application and writes it to the project's env file.
- Binaries for macOS, Linux and Windows, and installs through npm and Homebrew.

[Unreleased]: https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/releases/tag/v0.1.0
