# Error: leaktk command not found

The `leaktk` binary could not be found in your `PATH`. This error appears when
a git hook managed by leaktk fires but the `leaktk` command is not available in
the shell environment used by git.

## How to fix it

1. **Confirm leaktk is installed:**

   ```sh
   leaktk version
   ```

   If this fails, follow the [installation guide](../install.md).

2. **Confirm it is in your PATH:**

   ```sh
   which leaktk
   echo $PATH
   ```

   The directory containing the `leaktk` binary must appear in `$PATH`.

3. **Shell startup files:** Git hooks run in a non-interactive shell, so
   variables set only in `~/.bashrc` (interactive shells) may not be available.
   Add `leaktk` to a path that non-interactive shells pick up, such as
   `/usr/local/bin`, or set `PATH` explicitly in `/etc/environment`.

## Removing the hook

If you want to stop using leaktk hooks entirely, remove the hook file:

```sh
rm .git/hooks/pre-commit
```
