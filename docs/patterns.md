# Patterns

The LeakTK scanner fetches patterns from a pattern server to ensure it's always
using the most up-to-date rules for finding leaks. This document explains how
that works and how to configure it.

## Default Pattern Server

By default, LeakTK is configured to fetch patterns from the LeakTK pattern
repository hosted on GitHub:

- **URL:** `https://raw.githubusercontent.com/leaktk/patterns/main/target`

The scanner will automatically cache these patterns locally and refresh them
periodically to ensure you have the latest updates.

## Custom Pattern Server

For users who need to use their own set of patterns or host them in a private
environment, LeakTK can be configured to point to a custom pattern server.

Configuration can be done either through the main `config.toml` file or by
setting environment variables.

### Login

If your custom pattern server requires authentication, you can log in using the
`leaktk login` command. This will prompt you for an authentication token and
store it securely on your local machine.

```sh
leaktk login
```

Alternatively, you can provide the token directly via an environment variable or
in the configuration file, as shown in the examples below.

### Configuration Examples

#### Config File

You can specify the server URL token in your `config.toml`
file located at `$HOME/.config/leaktk/config.toml`.

```toml
[scanner.patterns.server]
# The URL to your custom pattern server.
# The path "/patterns/{scanner}/{version}" will be appended to this URL.
url = "https://patterns.example.com"
```

And then run `leaktk login` to provide set the auth token.

#### Environment Variables

You can also configure the pattern server using environment variables, which
will override any settings in the `config.toml` file.

```sh
# Set the URL for your custom pattern server
export LEAKTK_PATTERN_SERVER_URL="https://patterns.example.com"

# Set the authentication token
export LEAKTK_PATTERN_SERVER_AUTH_TOKEN="<YOUR_AUTH_TOKEN>"

# Run the scanner
leaktk scan ...
```
