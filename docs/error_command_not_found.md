# Error: leaktk command not found

The `leaktk` binary could not be found in your `PATH`. This error appears when
a hook or tool tries to execute leaktk but the `leaktk` command is not
available in the shell environment.

## Steps to resolve

1. **Confirm leaktk is installed:**

   ```sh
   leaktk version
   ```

   If this fails, follow the [installation guide](install.md).

2. **Ensure the directory it is installed in is in your PATH:**

   This should return the path to the directory that the command is installed
   under:

   ```sh
   echo "${PATH}" | grep -oF "$(dirname "$(command -v leaktk)")"
   ```

   If it doesn't, locate the directory that the leaktk command is installed in
   and ensure it is included in your enviornment's path variable. If you are
   unsure how, do a web search for "Set PATH variable on YOUR\_OS", replacing
   YOUR\_OS with the name of your operating sytem (e.g. Linux, Mac, Windows).

   After updating your PATH env variable, you may need to reload the environment
   that you were running the git commands in for the changes to apply. Or if
   working from a terminal, you can source your terminal's config file:

   ```sh
   if [ -n "${BASH_VERSION}" ]; then source ~/.bashrc; else source ~/.zshrc; fi
   ```

## Removing Git Hooks

If you want to stop using leaktk hooks entirely, remove the hook file:

```sh
rm .git/hooks/pre-commit
```

Or here is an example for removing it from all repos under a provided path:

```sh
find . -name 'pre-commit' -type f -path '*/hooks/pre-commit' 2> /dev/null \
  | xargs -I '{}' sh -c 'grep -E "\bleaktk\b" "{}" > /dev/null 2>&1 && rm -v "{}"'
```
