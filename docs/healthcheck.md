# Healthcheck

`healthcheck` checks project settings that help prevent credentials from being
committed. It uses the current directory when no project is provided.

```sh
# Report issues
leaktk healthcheck /path/to/project

# Apply safe fixes
leaktk healthcheck --fix /path/to/project

# Fail when an issue is found (for CI)
leaktk healthcheck --exit-code 1 /path/to/project
```

`--fix` only makes changes for findings it can fix safely. Running it more than
once will not add the same setting again.

Currently, `healthcheck` checks that `.env` is ignored by the project-root
`.gitignore`. The fix adds `.env` to `.gitignore`, creating that file when
needed.

The command uses the same `--format` options as `scan`. Each finding includes
the policy name, affected path, recommended fix, and whether it was fixed.
