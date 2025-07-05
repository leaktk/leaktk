# Scanner

Provides a consistent API around some existing scanning tools to integrate them
with the rest of the toolkit.

The scanner leverages
[Gitleaks](https://github.com/gitleaks/gitleaks)
internally because Gitleaks is an awesome tool, and we already have quite a few
[patterns](https://github.com/leaktk/patterns)
for it.

## Usage

```sh
# Listen for requests
leaktk listen < ./examples/requests.jsonl

# Run a single scan
leaktk scan 'https://github.com/leaktk/fake-leaks.git'
leaktk scan --kind JSONData '{"key": "-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----"}'
leaktk scan --kind JSONData '@/path/to/some-file.json'

# See more options
leaktk help
```

The scanner should always generate a response to each request even if there
were errors during the scan.

For most scans `leaktk scan [--kind=<kind>] <resource>` is enough, but more
information about the supported kinds of resources and specific options for
each resource can be found in the docs for [listen mode](./listen.md). The
options listed in that doc can be provided with the `--options` flag and
should be formated as a JSON string.
