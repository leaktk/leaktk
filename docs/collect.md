# Collector

This collects facts from supported sources and returns them as a CSV. This is
for gathering information needed for remediation and analysis.

## Supported Sources

- `AtlassianCloudAdmin`

Planned: GitHubOrganization GitHubEnterpriseAccount, LDAP, TBD...

## Usage

```
Collect facts about given source ids in the config and stream them to stdout.

Facts are structured as a CSV with a header row and one fact per line. Facts
are similar to RDF triples in that they have a subject (the eid), predicate
(kind), and object (value).

An entity ID should be treated as ephemeral between runs and is solely for
grouping facts. Entity ID 0 is a special ID used for mapping enum IDs to
values.

Usage:
  leaktk collect [flags] <source-id>...

Flags:
  -h, --help   help for collect
```

### Example

Example [config.toml](config.md):

```toml
[[sources]]
id = "example-atlassian-cloud-admin"
kind = "AtlassianCloudAdmin"
token = "REPLACE_WITH_CLOUD_ADMIN_TOKEN"
org_id = "REPLACE_WITH_CLOUD_ORG_ID"
```

Running command:

```sh
# NOTE: collect can take multiple source IDs
leaktk collect example-atlassian-cloud-admin
```

Example output:

```csv
eid,kind,value
0,0,"ID"
0,1,"Active"
0,2,"EmailAddress"
0,3,"EmailAddressVerified"
0,4,"EntityKind"
0,5,"Name"
0,6,"RelatedEntityID"
0,7,"SourceID"
0,8,"URL"
0,9,"Username"
1,0,"123456:1234abcd-12ab-34de-5678-12345abcdef0"
1,1,"true"
1,2,"user1@example.com"
1,3,"true"
1,4,"AtlassianCloudUser"
1,5,"John Smith"
1,7,"example-atlassian-org"
1,8,"https://home.atlassian.com/o/1234abcd-12ab-34de-5678-12345abcdef0/people/123456:2234abcd-12ab-34de-5678-12345abcdef0"
2,0,"123456:2234abcd-12ab-34de-5678-12345abcdef0"
2,1,"true"
2,2,"user2@example.com"
2,3,"true"
2,4,"AtlassianCloudUser"
2,5,"Jane Doe"
2,7,"example-atlassian-org"
2,8,"https://home.atlassian.com/o/2234abcd-12ab-34de-5678-12345abcdef0/people/123456:2234abcd-12ab-34de-5678-12345abcdef0"
```

## ID Mapping

The entity ID (eid) column is ephemeral and can change between runs. It's only
meant for grouping facts together as a single record.

The CSV will _always_ start with `0` eids that map kind enum numbers to their
string representations.

For example in the line:

```csv
1,2,"user1@example.com"
```

- 1 is the entity ID to group all the facts together for this specific user
- 2 indicates that the value is this record's email address

We know that 2 means email address because of the mapping line:

```csv
0,2,"EmailAddress"
```

Here's an example Python script parsing the output above (with the CSV data
from above snipped out for brevity):

```python
#!/usr/bin/env python3

import csv
import io
import json

data = """
eid,kind,value
...snipped for brevity...
""".strip()

records = {}
fields = {}

for fact in csv.DictReader(io.StringIO(data)):
    if fact["eid"] == '0':
        fields[fact["kind"]] = fact["value"]
    else:
        entity = records.setdefault(fact["eid"], {})
        field = fields[fact["kind"]]
        entity[field] = fact["value"]

print(json.dumps(list(records.values()), indent=2))
```

Result:

```json
[
  {
    "ID": "123456:1234abcd-12ab-34de-5678-12345abcdef0",
    "Active": "true",
    "EmailAddress": "user1@example.com",
    "EmailAddressVerified": "true",
    "EntityKind": "AtlassianCloudUser",
    "Name": "John Smith",
    "SourceID": "example-atlassian-org",
    "URL": "https://home.atlassian.com/o/1234abcd-12ab-34de-5678-12345abcdef0/people/123456:2234abcd-12ab-34de-5678-12345abcdef0"
  },
  {
    "ID": "123456:2234abcd-12ab-34de-5678-12345abcdef0",
    "Active": "true",
    "EmailAddress": "user2@example.com",
    "EmailAddressVerified": "true",
    "EntityKind": "AtlassianCloudUser",
    "Name": "Jane Doe",
    "SourceID": "example-atlassian-org",
    "URL": "https://home.atlassian.com/o/2234abcd-12ab-34de-5678-12345abcdef0/people/123456:2234abcd-12ab-34de-5678-12345abcdef0"
  }
]
```
