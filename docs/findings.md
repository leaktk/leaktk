# Findings

Simply put: findings are what the scanner found. The end of this document
includes a few examples of what LeakTK findings can look like.

## Responding to Findings

TODO - talk about determining if it's a [false positive](./false_positives.md)
and rough steps to remediate:

### Before You Begin

TODO - warning about destructive actions

### Steps

Rough outline (probably want to break this into separate or use expanding sections):

1. If exposed publically:
   1. Revoke credentials
   1. Make a backup of the resource containing the leak

1. If git:
   1. Clone a fresh copy of the repo to work on
   1. Purge secrets from the history
   1. If exposed publically:
      1. If GitHub: Steps to clean up remote
      1. If GitLab: Steps to clean up the remote
      1. If different git provider: recommend they read that provider's docs

1. If not git: TODO

## Findings Examples

The findings might be formatted a little differently depending on how leaktk is
invoked to tailor it for that specific use case.

For example when using LeakTK's Git commit hook, the findings might look
similar to this:

```
Findings:

- Description  : Generic Secret
  Commit:      :
  Path         : .env
  Line Number  : 1
  Encoding(s)  :

- Description  : AWS Secret Access Key
  Commit:      :
  Path         : .env
  Line Number  : 2
  Encoding(s)  :
```

**Note:** The secret is not included in the output by the hooks to avoid
spreading the secret further.

Or if running the scanner directly on Git repository with the standard JSON
output (expanded to make it easier to read):

```
{
  ...
  "results": [
    {
      "id": "DSFpsiJW0c4",
      "kind": "GitCommit",
      "secret": "SGcbDQvKIIskKSt4fbqp9b7LKknw+iYWF3nHSAf2B8M=",
      "match": "secret=\"SGcbDQvKIIskKSt4fbqp9b7LKknw+iYWF3nHSAf2B8M=\"",
      "context": "secret=\"SGcbDQvKIIskKSt4fbqp9b7LKknw+iYWF3nHSAf2B8M=\"",
      "entropy": 4.9534163,
      "date": "2026-04-22T19:14:02Z",
      "rule": { "id": "_-9w6-yrc-4", "description": "Generic Secret", "tags": [ "type:secret", "alert:repo-owner" ] },
      "contact": { "name": "Committer Name", "email": "commiter@example.com" },
      "location": { "version": "a4375489f1ac5c0011035b34971d860cd6191b2f", "path": "secret", "start": {"line": 1, "column": 1 }, "end": { "line": 1, "column": 53 } },
      "notes": {
        "commit_message": "Add secret",
        "gitleaks_fingerprint": "a4375489f1ac5c0011035b34971d860cd6191b2f:secret:_-9w6-yrc-4:1",
        "repository": "."
      }
    }
  ]
}
```
