# Scanner

Provides a consistent API around some existing scanning tools to integrate them
with the rest of the toolkit.

The scanner leverages
[Betterleaks](https://github.com/betterleaks/betterleaks)
internally because Gitleaks is an awesome tool, and we already have quite a few
[patterns](https://github.com/leaktk/patterns)
for it.

## Usage

```sh
# Listen for requests
leaktk listen < ./examples/requests.jsonl

# Scan a git repository (default kind)
leaktk scan 'https://github.com/leaktk/fake-leaks.git'

# Scan a container image
leaktk scan --kind ContainerImage 'quay.io/leaktk/fake-leaks:v1.0.1'

# Scan local files or directories
leaktk scan --kind Files /path/to/directory

# Scan JSON data inline or from a file
leaktk scan --kind JSONData '{"key": "-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----"}'
leaktk scan --kind JSONData '@/path/to/some-file.json'

# Scan arbitrary text
leaktk scan --kind Text 'some text to scan for secrets'

# Fetch and scan a URL
leaktk scan --kind URL 'https://raw.githubusercontent.com/leaktk/fake-leaks/main/keys/tls/server.key'

# See more options
leaktk help
```

The scanner should always generate a response to each request even if there
were errors during the scan.

For most scans `leaktk scan [--kind=<kind>] <resource>` is enough. The
supported kinds are:

- **ContainerImage**: Scan container images
- **Files**: Scan local filesystem paths
- **GitRepo**: Scan git repositories (default)
- **JSONData**: Scan JSON data for URLs to fetch and scan
- **Text**: Scan arbitrary text
- **URL**: Fetch and scan a URL

More information about each kind and specific options can be found in the docs
for [listen mode](listen.md). The options listed in that doc can be provided
with the `--options` flag and should be formatted as a JSON string.

## Auth

Current authentication support by resource kind:

| Resource Kind  | Support                                                    |
| -------------- | ---------------------------------------------------------- |
| ContainerImage | Local [containers-auth.json(5)][containers-auth]           |
| Files          | N/A                                                        |
| GitRepo        | Local [git credentials][git-credentials]                   |
| JSONData       | [Sources][sources] for `fetch_urls` option when applicable |
| Text           | N/A                                                        |
| URL            | [Sources][sources] when applicable                         |

[containers-auth]: https://github.com/podman-container-tools/container-libs/blob/HEAD/image/docs/containers-auth.json.5.md
[git-credentials]: https://git-scm.com/book/en/v2/Git-Tools-Credential-Storage
[sources]: config.md#sources
