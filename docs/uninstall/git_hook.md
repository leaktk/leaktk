## Removing Git Hooks

If you want to stop using leaktk hooks entirely, remove the hook file:

```sh
rm .git/hooks/pre-commit
```

Or here is an example for removing it from all repos under a provided path:

```sh
```

# Uninstalling Git Hooks

This guide walks you through the process of uninstalling
leaktk's supported [Git hooks](https://git-scm.com/docs/githooks).

| Hook Name       | Audience                  | Purpose                                       |
| --------------- | ------------------------- | --------------------------------------------- |
| git.pre-commit  | Developers                | Block creating new commits containing secrets |
| git.pre-receive | Git Server Administrators | Block git pushes containing secrets           |

## Prerequisites

This guide assumes you are working in a Unix-like environment (e.g. Mac, Linux) with the
`find`, `xargs`, and `rm` commands available on your system.

## Steps

For this guide we're going to use the git pre-commit hook, but the process is similar for the
other hooks.

1. Choose the hook you want to uninstall (in our case it's `git.pre-commit`).
1. Change directories in your terminal to the directory containing the git repository or multiple repositories containing the hook.
1. Run this command to remove the hook from all repositories under your current directory
   ```sh
   find . -name 'pre-commit' -type f -path '*/hooks/pre-commit' 2> /dev/null \
     | xargs -I '{}' sh -c 'grep -E "^#\s*CreatedBy:\s*leaktk" "{}" > /dev/null 2>&1 && rm -v "{}"'
   ```
1. If you installed the hook with the `--user-template-dir` flag, you can uninstall it from there using:
   ```sh
   git config --path init.templateDir \
     | xargs -I '{}' sh -c 'grep -E "^#\s*CreatedBy:\s*leaktk" "{}/hooks/pre-commit" > /dev/null 2>&1 && rm -v "{}/hooks/pre-commit"'
   ```

You should see output similar to this when uninstalling it from the repos:

```
...
removed './leaktk-hack/.git/hooks/pre-commit'
removed './leaktk-leaktk/.git/hooks/pre-commit'
removed './leaktk-patterns/.git/hooks/pre-commit'
```

And something like this when uninstalling it from your Git template directory:

```
removed '/home/user/.config/git/template/hooks/pre-commit'
```
