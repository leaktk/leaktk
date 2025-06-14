# Listen

`listen` mode, LeakTK listens on stdin, responds on stdout, and logs to
stderr. It reads one request per line and sends one response per line in
[JSONL](https://jsonlines.org/). It should always generate a response to each
request even if there were errors.


## Request/Response formats

Notes about the formats below:

* Scan requests should be sent as [JSON lines](https://jsonlines.org/).
* The examples below are pretty printed to make them easier to read.
* Only the values in the `"options"` sections are optional.

### GitRepo

#### Request

Remote:

```json
{
  "id": "85V5qL7x_bY",
  "kind": "GitRepo",
  "resource": "https://github.com/leaktk/fake-leaks.git",
  "options": {
    "branch": "main",
    "depth": 1,
    "proxy": "http://squid.example.com:3128",
    "since": "2020-01-01"
    "local": false,
  }
}
```

Local:

```json
{
  "id": "db0c21127a6a849fdf8eeae65d753275f3a26a33649171fa34af458030744999",
  "kind": "GitRepo",
  "resource": "./fake-leaks.git",
  "options": {
    "branch": "main",
    "depth": 1,
    "local": true,
    "unstaged": false,
    "staged": true,
  }
}
```

The `options` above are not required and some combined (e.g. `depth` and `since`)
may cause issues. Refer to the details below to better understand the options
and [git's docs](https://git-scm.com/) for knowing what can be combined.

#### Request Options

**branch**

Sets `--branch` and `--single-branch` during git clone.

* Type: `string`
* Default: excluded

**depth**

Sets `--depth` during a git clone and can limit the commits during a local
scan if `single_branch` is set to true.

* Type: `int`
* Default: excluded

**local**

Scans a local git repo instead of fetching a remote one. When listening
for jsonl requests this must be explicitly set. When running single scans
it's inferred by the resource.

* Type: `bool`
* Default: `false`

**since**

Is a date formatted `yyyy-mm-dd` used for filtering commits. Sets
`--shallow-since` during a clone.

* Type: `string`
* Default: excluded

**staged**

Only scan staged changes. This takes priority over `unstaged` and is ignored
for non-local repos.

* Type: `bool`
* Default: `false`

**proxy**

A URL for a http proxy. Sets `--config http.proxy` during the
clone.

* Type: `string`
* Default: excluded

**priority**

Sets the request priority. Higher priority items will be scanned first.

* Type: `int`
* Default: `0`

**unstaged**

Scan current changes rather than the history. `staged` takes priority over this
and is ignored for non-local repos.

* Type: `bool`
* Default: `false`

#### Response

```json
{
  "id": "rMr0GAfwYwd",
  "request": {
    "id": "85V5qL7x_bY",
    "kind": "GitRepo",
    "resource": "http://github.com/leaktk/fake-leaks.git"
  },
  "logs": [],
  "results": [
    {
      "id": "IDsQnA1t6Em",
      "kind": "GitCommit",
      "secret": "-----BEGIN EC PRIVATE KEY-----\n...snip...\n-----END EC PRIVATE KEY-----",
      "match": "-----BEGIN EC PRIVATE KEY-----\n...snip...\n-----END EC PRIVATE KEY-----",
      "entropy": 5.763853,
      "date": "2023-08-04T12:21:12Z",
      "rule": {
        "id": "3fk1rL-aRiw",
        "description": "Private Key",
        "tags": [
          "group:leaktk-testing",
          "alert:repo-owner",
          "type:secret"
        ]
      },
      "contact": {
        "name": "The Committer",
        "email": "user@example.com"
      },
      "location": {
        "version": "d5bcb89de5311aaa688cb23d8d2d78cf7cd74f1f",
        "path": "keys/tls/another-key.key",
        "start": {
          "line": 1,
          "column": 1
        },
        "end": {
          "line": 6,
          "column": 29
        }
      },
      "notes": {
        "message": "Add another key"
      }
    }
  ]
}
```

### Text

Scan arbitrary strings

#### Request

```json
{
  "id": "OBZMFe4gBnx",
  "kind": "Text",
  "resource": "-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----"
}
```

### JSONData

This allows you to scan various JSON structures for secrets.

#### Request

```json
{
  "id": "OBZMFe4gBnt",
  "kind": "JSONData",
  "resource": "{\"some\": {\"key\": \"-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----\"}}"
}
```

Note: the `resource` here is a string containing escaped JSON data.

#### Request Options

**fetch_urls**

This is a list of `:` separated paths down the JSON data in which URL fetching
is allowed. By default it's disabled and basic `*` and recursive `**` path
globbing is supported.

If one of the values in an allowed path is a URL, it will be fetched and
treated as the content at that path in the data structure. This is like running
a URL scan. If the response is JSON it can be traversed too, but it won't fetch
further URLs.

* Type: `string`
* Default: `""`

Examples:

- `**` fetch any URLs found
- `users/*/url` fetch things like `user/foo/url` and `user/1/url`
- `link` only fetch a url if it exists at a top level `link` field
- `**/url` fetch any url who's key is `url` at any level
- `**/users/*/url` like the other users example above but at any level
- `attachments/**` fetch any urls under the attachments path
- `files/**:links/**` fetch anything under files or links

**NOTE:** It is **NOT** recommended to enable this option unless you trust
the JSON and the URLs you are providing to the scanner.

Gitleaks config files (e.g. `.gitleaks.toml`, `.gitleaksignore`,
`.gitleaksbaseline`) as top level keys that will be considered during the
scan. The [examples/requests.jsonl](../examples/requests.jsonl) has an
example of including a `.gitleaks.toml` with a JSONData scan.

**priority**

Sets the request priority. Higher priority items will be scanned first.

* Type: `int`
* Default: `0`

#### Response

```json
{
  "id": "tMd_Av-ZTFP",
  "request": {
    "id": "Dg9hdRRdd2M",
    "kind": "JSONData",
    "resource": "{\"some\":{\"key\": \"-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----\"}}"
  },
  "logs": [],
  "results": [
    {
      "id": "ZwaaqRjgNgk",
      "kind": "JSONData",
      "secret": "-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----",
      "match": "-----BEGIN PRIVATE KEY-----c5602d28d0f21422dfc7b572b17e6b138c1b49fd7f477d4c5c961e0756f1ff70-----END PRIVATE KEY-----",
      "entropy": 4.490795,
      "date": "",
      "rule": {
        "id": "3fk1rL-aRiw",
        "description": "Private Key",
        "tags": [
          "group:leaktk-testing",
          "alert:repo-owner",
          "type:secret"
        ]
      },
      "contact": {
        "name": "",
        "email": ""
      },
      "location": {
        "version": "",
        "path": "some/key",
        "start": {
          "line": 0,
          "column": 1
        },
        "end": {
          "line": 0,
          "column": 116
        }
      },
      "notes": {}
    }
  ]
}
```

Note: the `path` here is formatted like a file path. For arrays, the element's
index is used.

### Files

This allows you to scan files and directories.

#### Request

```json
{
  "id": "04mTfB9Lxd1",
  "kind": "Files",
  "resource": "/path/to/fake-leaks/keys/tls"
}
```

Note: the `resource` can be either a single file or a directory.

#### Request Options

**priority**

Sets the request priority. Higher priority items will be scanned first.

* Type: `int`
* Default: `0`

#### Response

```json
{
  "id": "cAdqKrap5Hu",
  "request": {
    "id": "A18dGVI86dm",
    "kind": "Files",
    "resource": "/path/to/fake-leaks/keys/tls"
  },
  "logs": [],
  "results": [
    {
      "id": "oOp7aV7LASW",
      "kind": "General",
      "secret": "-----BEGIN EC PRIVATE KEY-----\n...snip...\n-----END EC PRIVATE KEY-----",
      "match": "-----BEGIN EC PRIVATE KEY-----\n...snip...\n-----END EC PRIVATE KEY-----",
      "entropy": 5.763853,
      "date": "",
      "rule": {
        "id": "3fk1rL-aRiw",
        "description": "Private Key",
        "tags": [
          "group:leaktk-testing",
          "alert:repo-owner",
          "type:secret"
        ]
      },
      "contact": {
        "name": "",
        "email": ""
      },
      "location": {
        "version": "",
        "path": "another-key.key",
        "start": {
          "line": 0,
          "column": 1
        },
        "end": {
          "line": 5,
          "column": 29
        }
      },
      "notes": {}
    }
  ]
}
```

Note: the `path` is relative to the resource provided. If the resource is the
path to the file itself, then path will be empty.

### URL

This allows you to pull remote content to scan. It also has basic awareness
of the response `Content-Type`. If the response has a content type of
`application/json` it will be parsed as a `JSONData` request. Else it will
be parsed as a `Files` request.

#### Request

```json
{
  "id": "r-3Y5aAIckV",
  "kind": "URL",
  "resource": "https://raw.githubusercontent.com/leaktk/fake-leaks/main/keys/tls/server.key"
}
```

#### Request Options

**priority**

Sets the request priority. Higher priority items will be scanned first.

* Type: `int`
* Default: `0`

#### Response

```json
{
  "id": "r-3Y5aAIckV",
  "request": {
    "id": "DrlFMAs-m-D",
    "kind": "URL",
    "resource": "https://raw.githubusercontent.com/leaktk/fake-leaks/main/keys/tls/server.key"
  },
  "logs": [],
  "results": [
    {
      "id": "Czoctar4oKt",
      "kind": "General",
      "secret": "-----BEGIN PRIVATE KEY-----\n...snip...\n-----END PRIVATE KEY-----",
      "match": "-----BEGIN PRIVATE KEY-----\n...snip...\n-----END PRIVATE KEY-----",
      "entropy": 6.0285063,
      "date": "",
      "rule": {
        "id": "3fk1rL-aRiw",
        "description": "Private Key",
        "tags": [
          "group:leaktk-testing",
          "alert:repo-owner",
          "type:secret"
        ]
      },
      "contact": {
        "name": "",
        "email": ""
      },
      "location": {
        "version": "",
        "path": "",
        "start": {
          "line": 2,
          "column": 2
        },
        "end": {
          "line": 29,
          "column": 26
        }
      },
      "notes": {}
    }
  ]
}
```

Note: the `path` will be blank when using a `Files` type scan and it will
use the same logic as `JSONData` when the response's content type is
`application/json` (i.e. it will be the path down the traversed keys).

### Container Image

This allows you to pull a remote container image to scan. It unpacks and scans
the Image, Config and Manifest.

#### Request

```json
{
  "id": "GTykQmSnoio",
  "kind": "ContainerImage",
  "resource": "quay.io/leaktk/fake-leaks:v1.0.1"
}
```

#### Request Options

**arch**

Provide a preferred architecture

* Type: `string`
* Default: excluded

**depth**

Sets the number of layers to download and scan, starting from the top

* Type: `int`
* Default: 0 (All layers scanned)

**exclusions**

Sets a list of RootFS Layer hashes to exclude from scanning

* Type: `[]string`
* Default: excluded

Example `"options":{"exclusions":["2b84bab8609aea9706783cda5f66adb7648a7daedd2650665ca67c717718c3d1"]}`

**priority**

Sets the request priority. Higher priority items will be scanned first.

* Type: `int`
* Default: `0`

**since**

Is a date formatted `yyyy-mm-dd` used for filtering layers based on provided history. History is optional
so not all images will have the information.

* Type: `string`
* Default: excluded

#### Response
```json
{
  "id": "X71VJDDYKUc",
  "request": {
    "id": "e6jlVhrbFBq",
    "kind": "ContainerImage",
    "resource": "quay.io/leaktk/fake-leaks:v1.0.1"
  },
  "results": [
    {
      "id": "1JYuHIigWRd",
      "kind": "ContainerLayer",
      "secret": "-----BEGIN PRIVATE KEY-----\n...snip...\n-----END PRIVATE KEY-----",
      "match": "-----BEGIN PRIVATE KEY-----\n...snip...\n-----END PRIVATE KEY-----",
      "entropy": 6.0285063,
      "date": "",
      "rule": {
        "id": "3fk1rL-aRiw",
        "description": "Private Key",
        "tags": [
          "group:leaktk-testing",
          "alert:repo-owner",
          "type:secret"
        ]
      },
      "contact": {
        "name":"Fake Leaks",
        "email":"fake-leaks@leaktk.org"
      },
      "location": {
        "version": "bd34309759a381a850f60d90bb1adc2a1756bbdf11d746c438c8706a13c63f66",
        "path": "fake-leaks/base64-encoded",
        "start": {
          "line": 2,
          "column": 2
        },
        "end": {
          "line": 10,
          "column": 13
        }
      },
      "notes": {}
    }
  ]
}
```

