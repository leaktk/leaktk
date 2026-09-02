# Configuration

## Config File Format

LeakTK's config can be changed by adjusting values in a config
[TOML](https://toml.io/en/) in one of the following locations following order
of precedence:

1. The `LEAKTK_CONFIG_PATH` environment variable
1. `--config <some path>`
1. `${XDG_CONFIG_HOME}/leaktk/config.toml` if it exists
1. `/etc/leaktk/config.toml` if it exists
1. The default config defined in [config.go](../pkg/config/config.go)

The following environment variables take precedence over the config when set:

| Environment Variable               | Overrides                            |
| ---------------------------------- | ------------------------------------ |
| `LEAKTK_LOGGER_LEVEL`              | `logger.level`                       |
| `LEAKTK_PATTERN_SERVER_AUTH_TOKEN` | `scanner.patterns.server.auth_token` |
| `LEAKTK_PATTERN_SERVER_URL`        | `scanner.patterns.server.url`        |
| `LEAKTK_SCANNER_AUTOFETCH`         | `scanner.patterns.autofetch`         |

## Config Sections

The following covers different sections of the config. Items in the config
should have valid defaults and customizing the config is not needed for most
use cases.

**NOTE**: This config file format is still in the draft stages and will likely
change (e.g. the patterns section may become a top level `server` section in
the future).

### Logger

Configure the verbosity of the logger.

```toml
[logger]
# Valid Values: "ERROR", "WARN", "INFO", "DEBUG", "TRACE"
# Default: "INFO"
level = "INFO"
```

### Scanner

Default scanner settings:

```toml
[scanner]
# How long a scan can run before it's canceled
scan_timeout = 0 # 0 means no timeout

# How deep should the scanner decode encoded values
max_decode_depth = 8 # 0 means no decoding

# Allow scanning into nested archives up to this depth
max_archive_depth = 8 # 0 means no decoding

# How many commits can be scanned
max_scan_depth = 0 # 0 means no max depth.

# How many scans can happen at once
scan_workers = 1

# How many items the scan queue can hold in it before it blocks (0 default means non-blocking)
max_scan_queue_size = 1

# How many items the response queue can hold in it before it blocks (0 default means non-blocking)
max_response_queue_size = 1

# The full path to where the scanner should store files, cloned repositories, etc
# for better performance mount a tmpfs at this location
# workdir = "/tmp/leaktk/scanner" # This defaults to ${XDG_CACHE_HOME}/leaktk/scanner

# Allow local scans on listen
allow_local = true
```

The `scanner.patterns` table configures how LeakTK manages its patterns.
These are the default settings:

```toml
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
# 1) LEAKTK_PATTERN_SERVER_AUTH_TOKEN environment variable
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
# 1) LEAKTK_PATTERN_SERVER_URL environment variable
# 2) Fall back on "https://raw.githubusercontent.com/leaktk/patterns/main/target"
```

### Formatter

This sets the default for the `--format` flag.

```toml
[formatter]
# Valid values: "CSV", "HUMAN", "JSON", "TOML", "YAML"
format = "JSON"
```

### Sources

The sources are used by [collect][collect] for gathering facts and by other
parts of LeakTK for auth.

Supported kinds:

| Kind                | Used By                          |
| ------------------- | -------------------------------- |
| AtlassianCloudAdmin | [collect][collect], [scan][scan] |
| AtlassianCloudJira  | [scan][scan]                     |
| (More planned)      |                                  |

[collect]: collect.md
[scan]: scan.md

By default no sources are configured.

Example configurations:

```toml
#
# Example Atlassian Cloud Admin Source
#

[[sources]]
# Defines the source kind
kind = 'AtlassianCloudAdmin'

# A unique ID used to reference the source from the command line
id = 'my-atlassian-cloud-admin'

# ID of your Atlassian Cloud Admin Org
org_id = 'org-id-org-id-org-id-org-id'

# Atlassian Cloud Admin API token for your org
token = '...'

#
# Example Atlassian Cloud Jira Source
#

[[sources]]
# Defines the source kind
kind = 'AtlassianCloudJira'

# A unique ID used to reference the source from the command line
id = 'my-cloud-jira'

# The URL that the instance is located at
base_url = 'https://example.atlassian.net'

# ID of your Atlassian Cloud Jira Org
org_id = 'org-id-org-id-org-id-org-id'

# The username and password used for basic auth to the API
username = '...'
password = '...'
```
