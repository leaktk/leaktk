# Redactor

Provides tools for redacting secrets.

## Usage

```
USAGE
    leaktk redact --kind Stdio [flags]

DESCRIPTION
    Redact secrets from the provided target

    --kind=Stdio
        Reads data in chunks on stdin, redacts found secrets, and prints it to
        stdout.

    --redaction-mark='*'
        Replace each byte of the match with the redaction mark. The default is '*'

    --redaction-word=''
        Repalce each match with the redaction word instead of the redaction mark. This
        Takes precidence over redaction-mark if provided.
```

## Roadmap

> :page_facing_up: **NOTE:** The order may change here depending on needs.

- [x] Stdio redaction support
- [ ] Integration into [posix.stdio hook](https://github.com/leaktk/leaktk/issues/272)
- [ ] Text & Files redaction support
- [ ] Redact using existing scan results
- [ ] GitRepo redaction support
- [ ] GitHub cleanup automation
- [ ] GitLab cleanup automation
- [ ] JiraIssue cleanup automation
- [ ] JSONData redaction support
- [ ] Text redaction support
- [ ] GCS redaction support
- [ ] S3 redaction support
- [ ] Container redaction support
- [ ] TBD

