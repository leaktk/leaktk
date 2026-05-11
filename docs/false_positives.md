# False Positives

A false positive is a finding that poses **no risk** to an individual or
organization.

See [Understanding Finding Types][1] for more information about what
constitutes a false positive vs true positive or benign positive.

## Ignoring False Positives

> **🗒️ NOTE: If the finding is NOT a false positive, do not ignore it!**
>
> Follow the [remediation steps][2] to mitigate the risk.

### Option 1: Make examples look less real

> **🗒️ NOTE: This won't stop the scanner from alerting on previous commits without rewriting history**
>
> This is best used as a way to prevent initial false positives.

> **⚠️  WARNING: If you do decide to rewrite history, be aware that it is a destructive action**

If feasible, make example, placeholder, test, and dummy values look less real.
This could include things like putting `EXAMPLE` in the value or `XXXX` or
other similar values to ensure it doesn't look like a real secret.

### Option 2: Add a supported ignore comment on the line containing the secret

> **🗒️ NOTE: This won't stop the scanner from alerting on previous commits without rewriting history**
>
> This is best used as a way to prevent initial false positives.

> **⚠️  WARNING: If you do decide to rewrite history, be aware that it is a destructive action**

Similar to option 1, this communicates to the scanner and others that the example, placeholder, test, or dummy value is not a real secret.

**Supported Comments:**

- `notsecret` or `not secret` can be anywhere in the line, is case insensitive and the space is optional.
- `gitleaks:allow` or `betterleaks:allow` given that the scanner is based on betterleaks, both of these tags are supported.

There is also support to some extent for `nosec` and `noqa` comments but those are currently defined for certain rules and not broadly like the supported ones above.

### Option 3: Add a `.gitleaks.toml`

> **🗒️ NOTE: This option works best for ignoring historical false positives**

> **⚠️  WARNING: Be careful not to add allowlists that are too broad**
>
> If the allowlist is too broad, it increases the risk of ignoring real secrets.

LeakTK will look for a `.gitleaks.toml` file at the root of Git repositories
and directories during a scan and merge any global allowlists it finds with its
own. Allowlists are written in [TOML][3] and we currently support the [gitleaks
v8 allowlist format][4].

If multiple branches are scanned (the default if not providing a branch), the
`.gitleaks.toml` on the [HEAD](https://git-scm.com/docs/gitglossary#def_HEAD)
will be used. Otherwise the `.gitleaks.toml` on the provided branch will be
used.

LeakTK will **ignore**:

- Files with any config errors
- All `[[rules]]` tables and sub-tables
- The `[extend]` table

**Example Allowlists:**

By default any field can match for an allowlist to apply, but if you want to
require multiple fields to be true for an item to be allowed, you can set the
`condition` field to AND like it is in one of the examples below.

```toml
# Ignore findings containing 'p4$$w0rd' (case insensitive) in the config and
# response files in test.

[[allowlists]] # Note the double braces and plural allowlists
# AND here means that all of the "fields" defined much have match here
condition = "AND"
# stopwords are not regex and are just case insensitive string matches that
# must be contained in the finding's secret
stopwords = [
    'p4$$w0rd',
]
paths = [
    # Triple single quotes avoids needing to do TOML string escaping on top of
    # any other regex escapes needed
    '''^tests\/config\.json$''',
    # "condition" is at the "field" level so any of these paths have to match
    # with the stopword above for the rule to apply
    '''^tests\/response\.json$''',
]

# Ignore anything that has the value '# nosec' at the end of the line
[[allowlists]]
# This sets the regex matcher to match against the "line" field rather than the
# finding's "match" field or the "secret" field.
regexTarget = 'line'
regexes = [
    # The '\s*' matches zero or more spaces
    '''#\s*nosec\s*$''',
]
```

[1]: findings.md#understanding-finding-types
[2]: findings.md#remediation-steps
[3]: https://toml.io/en/
[4]: https://github.com/gitleaks/gitleaks/blob/67fcd836b3448e2d7bb928aa76082fc4ac894137/README.md#configuration
