# Changelog

## 1.2.0

### Changed

- Page caching is enabled by default. Disable it with `--disable-cache` (`MDWEB_DISABLE_CACHE`); the previous `--cache` / `-c` flags were removed. The navigation sidebar is costly to rebuild on every request, which is why caching is now the default.
