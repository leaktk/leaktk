# Findings

Findings are the secrets, credentials, and sensitive data that LeakTK's scanner detects in your code, repositories, or other scanned resources.

This document covers:

- How to interpret and respond to findings
- Understanding different finding types (true positives, benign positives, false positives)
- Remediation steps for exposed secrets
- Example finding formats from different scan modes

See [Findings Examples](#findings-examples) below for sample output formats.


## Responding to Findings

When LeakTK reports a finding, you need to determine what type of finding it is
before deciding how to respond. Understanding the distinction between different
finding types is crucial for effective remediation.

### Understanding Finding Types

**What is a "Secret" in This Context?**

A secret is any piece of sensitive information that, if exposed, could pose a risk
to an individual or organization. This includes but is not limited to:

- API keys and tokens
- Database credentials
- Private keys and certificates
- OAuth tokens and session identifiers
- Encryption keys
- Personal Identifiable Information (PII)
- Internal URLs or system configurations that reveal architecture

**The sensitivity of a secret is highly contextual.** What's sensitive in one
environment may not be in another, but err on the side of caution.

**Finding Types:**

1. **True Positive**: A real secret that should not be exposed.
   - Production credentials
   - Personal Identifiable Information (PII)
   - Internal URLs or system configurations that reveal architecture
   - Valid API keys with active permissions
   - Real private keys
   - **Important**: Pre-production credentials (dev, qa, staging, test) are still
     secrets! They can provide attackers with valuable information about your
     systems and may have more permissions than intended.

2. **Benign Positive**: A real secret that's intentionally present and acceptable
   - Revoked or expired credentials kept for historical reference
   - Information stored somewhere where it's allowed to be stored (e.g.
     Internal URLs in an internal Git repository)

3. **False Positive**: Not actually a secret, just looks like one
   - Dummy values in examples (e.g., `password: "your-password-here"`)
   - Random strings that match secret patterns but aren't secrets
   - Hash values or checksums that look like keys but aren't keys
   - UUIDs or other identifiers that aren't actually sensitive
   - Example secrets in documentation showing proper format
   - Public API keys that are meant to be public (though these should be clearly
     marked)

**How to Determine the Finding Type:**

1. **Examine the context**: Where did this appear? Is it in production code,
   tests, documentation, or examples?

2. **Verify if it's real**: Does this credential actually work? Can you use it
   to access a system?

3. **Assess the risk**: Even if it's "just" a dev credential, what could an
   attacker do with it?

4. **Check the intent**: Was this meant to be committed? Is it documented as
   an example?

**Next Steps Based on Finding Type:**

- **True Positive**: Follow the remediation steps below immediately
- **Benign Positive**: Consider if it should remain or be better documented;
  you may want to add it to an ignore list (see [false positives](false_positives.md))
- **False Positive**: Add appropriate ignore rules to prevent future noise
  (see [false positives](false_positives.md))

### Remediation Steps

> **⚠️  WARNING: Destructive Actions Ahead**
>
> Redacting sensitive information from any source is a **destructive action** that
> permanently modifies or removes data. Whether you're rewriting Git history,
> editing files, or cleaning up container images, these operations cannot be easily
> undone.
>
> **Before proceeding with remediation:**
> - **Back up the resource locally** before making any changes
> - Verify your backups are complete and accessible
> - Test your remediation process on a copy first if possible
> - Document what you're changing and why
>
> If something goes wrong during remediation, having a backup ensures you can
> recover the original state and try again.


Rough outline (probably want to break this into separate or use expanding sections):

#### Step 1: Alert your Cyber Security Incident Response Team (CSIRT)

> **🗒️ NOTE: This step only applies to exposed company secrets**

If the exposed secrets contain company credentials or data, it is important that
you promptly notify your Cyber Security Incident Response Team (CSIRT).

If you are unsure how, try searching your company's intranet for things like:

- "Report Cybersecurity Concern"
- "Report Security Incident" 

#### Step 1: Revoke all publically exposed credentials

> **🗒️ NOTE: This step only applies to publically exposed credentials**

It is important that exposed credentials be rotated as soon as possible to
minimize the risk of abuse. The specific steps for rotating credentials will
vary depending on what the credentials. 

To find guides for rotating credentials:

- Search "Rotate _Credential Type_"
- Check out sites like [How to Rotate](https://howtorotate.com/)
- Ask your CSIRT for help

#### Step 3: Make a local backup 

TODO

#### Step 4: Redact the secrets

> **⚠️  WARNING: This is a destructive action**
> Make sure you completed the previous backup step before continuing

> **🗒️ NOTE: There may be cases where redaction is not neccessary**
> In cases where redaction may cause more harm than good work with
> your CSIRT to determine the right course of action. 

TODO

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
