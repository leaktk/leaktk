# Patterns

The LeakTK scanner fetches patterns from a LeakTK server to ensure it's always
using the most up-to-date rules for finding leaks. This document explains how
that works and how to configure it.

## Default LeakTK Server

By default, LeakTK is configured to fetch patterns from the LeakTK pattern
repository hosted on GitHub:

- **URL:** `https://raw.githubusercontent.com/leaktk/patterns/main/target`

The scanner will automatically cache these patterns locally and refresh them
periodically to ensure you have the latest updates.

## Custom LeakTK Server

For users who need to use their own set of patterns or host them in a private
environment, LeakTK can be configured to point to a custom LeakTK server.

Configuration can be done either through the main `config.toml` file or by
setting environment variables.

### Login

If your custom LeakTK server requires authentication, you can log in using the
`leaktk login` command. There are several ways to authenticate:

```sh
# Browser OAuth login
leaktk login --web

# Browser login with a specific server
leaktk login https://patterns.example.com --web

# Provide a token directly
leaktk login --token=<YOUR_AUTH_TOKEN>

# Prompt for a token interactively
leaktk login
```

With `--web`, the CLI will check the server for OAuth support, open your
browser for authentication, and store the resulting token. If a browser
can't be opened, the URL will be printed so you can open it manually.

Alternatively, you can provide the token directly via an environment variable or
in the configuration file, as shown in the examples below.

### Autologin

If `autologin` is enabled in the config (or via the `LEAKTK_AUTOLOGIN` env
var), the scanner will automatically attempt a browser login when a custom
LeakTK server returns a 401. This is useful for endpoints where tokens expire
periodically. It defaults to `false` to avoid impacting hooks and other
not interactive use cases.

```toml
[scanner.patterns]
autologin = true
```

Or using environment variable:

```sh
export LEAKTK_AUTOLOGIN=true
```

### Configuration Examples

#### Config File

You can specify the server URL token in your `config.toml`
file located at `$HOME/.config/leaktk/config.toml`.

```toml
[scanner.patterns.server]
# The URL to your custom LeakTK server.
# The path "/patterns/{scanner}/{version}" will be appended to this URL.
url = "https://patterns.example.com"
```

And then run `leaktk login` to set the auth token.

#### Environment Variables

You can also configure the LeakTK server using environment variables, which
will override any settings in the `config.toml` file.

```sh
# Set the URL for your custom LeakTK server
export LEAKTK_PATTERN_SERVER_URL="https://patterns.example.com"

# Set the authentication token
export LEAKTK_PATTERN_SERVER_AUTH_TOKEN="<YOUR_AUTH_TOKEN>"

# Run the scanner
leaktk scan ...
```

## Refreshing patterns

Sometimes you may want to refresh your local patterns cache before they expire
on their own. To do this run `leaktk refresh`. This will fetch fresh patterns
from the currently configured LeakTK server. Given that this fetch is manual,
this will still fetch patterns even if `LEAKTK_SCANNER_AUTOFETCH` is disabled.
