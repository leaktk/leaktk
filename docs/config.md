# Configuration

## Config File Format

LeakTK's config can be changed by adjusting values in a config
[TOML](https://toml.io/en/) in one of the following locations following order
of precedence:

1. The `LEAKTK_CONFIG_PATH` env var
1. `--config <some path>`
1. `${XDG_CONFIG_HOME}/leaktk/config.toml` if it exists
1. `/etc/leaktk/config.toml` if it exists
1. The default config defined in [config.go](../pkg/config/config.go)

There are also several env vars that take precedence over the other config
settings if they're set:

- `LEAKTK_LOGGER_LEVEL` - set the level of the logger
- `LEAKTK_PATTERN_SERVER_AUTH_TOKEN` - the pattern server token
- `LEAKTK_PATTERN_SERVER_URL` - the pattern server URL
- `LEAKTK_SCANNER_AUTOFETCH` - whether the scanner can auto fetch patterns or
  any other items it may need to do the scan (except for the resource being
  scanned)

## Example Config

All items in the config should have sane defaults and customizing the config
is not needed for most use cases

**NOTE**: This config file format is still in the draft stages and will likely
change.

```toml
[formatter]

# Valid values: "CSV", "HUMAN", "JSON", "TOML", "YAML"
format = "JSON"

[logger]

# Valid Values: "ERROR", "WARN", "INFO", "DEBUG", or "TRACE"
level = "INFO"

[scanner]
# How long a clone can run before it's canceled
clone_timeout = 0 # 0 means no timeout
# How deep should the scanner decode encoded values
max_decode_depth = 8 # 0 means no decoding
# Allow scanning into nested archives up to this depth
max_archive_depth = 8 # 0 means no decoding
# How many commits can be scanned
max_scan_depth = 0 # 0 means no max depth.
# How many scans can happen at once
scan_workers = 1
# The full path to where the scanner should store files, clone repos, etc
# for better performance mount a tmpfs at this location
# workdir = "/tmp/leaktk/scanner" # This defaults to ${XDG_CACHE_HOME}/leaktk/scanner
# Allow local scans on listen
allow_local = true

[scanner.patterns]
# Tells the scanner if it can fetch pattenrs or not
autofetch = true
# How long until the scanner refuses to use the cached patterns
expired_after = 604800 # 7 days
# How long until the scanner tries to fetch patterns if autofetch is allowed
refresh_after = 43200 # 12 hours

# Configure the gitleaks patterns. These generally don't need to be tweaked
# unless you have a special use case
# [scanner.patterns.gitleaks]
# config_path = where to store the config
# version = which version of the config to fetch

[scanner.patterns.server]
# This defines the auth bearer token sent to the server.
# auth_token = "<insert auth token here>"
# The following sources will override this setting
#
# 1) LEAKTK_PATTERN_SERVER_AUTH_TOKEN env var
# 2) ~/.config/leaktk/pattern-server-auth-token # set by the login command
# 3) /etc/leaktk/pattern-server-auth-token
#
# If none of the above are defined, no Authorization header is sent to the pattern
# server.

# The URL to a pattern server.
# The path "/patterns/{scanner}/{version}" will be appended to this URL
url = "https://raw.githubusercontent.com/leaktk/patterns/main/target"
# If this value is not set then the following sources will be checked in this order:
#
# 1) LEAKTK_PATTERN_SERVER_URL env var
# 2) Fall back on "https://raw.githubusercontent.com/leaktk/patterns/main/target"
```
