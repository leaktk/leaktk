# Installing Git Hooks

This guide walks you through the process of installing
leaktk's supported [Git hooks](https://git-scm.com/docs/githooks).

| Hook Name       | Audience                  | Purpose                                       |
| --------------- | ------------------------- | --------------------------------------------- |
| git.pre-commit  | Developers                | Block creating new commits containing secrets |
| git.pre-receive | Git Server Administrators | Block git pushes containing secrets           |

## Prerequisites

This guide requires that you have the `leaktk` command installed. If you do not already have
it installed, see the main [installation guide](../install.md).

## Steps

For this guide we're going to use the git pre-commit hook, but the process is similar for the
other hooks.

1. Choose the hook you want to install (in our case it's `git.pre-commit`).
1. Decide where you want to install it (this example will assume all Git repositories under your home directory: `"${HOME}"`).
1. Decide if you want to replace any existing hooks (this example will assume yes so we'll add: `--force`)
1. Decide if you want it to be enabled for new repos by default (this example assumes yes: `--user-template-dir`).
1. Run the install command with the proper flags set:
   ```sh
   leaktk install hook git.pre-commit --force --user-template-dir --path "${HOME}"
   ```
You should see output similar to this:

```
...
[INFO] installed hook: hook="git.pre-commit" path="/home/user/Workspace/leaktk-hack/.git/hooks/pre-commit"
[INFO] installed hook: hook="git.pre-commit" path="/home/user/Workspace/leaktk-leaktk/.git/hooks/pre-commit"
[INFO] installed hook: hook="git.pre-commit" path="/home/user/Workspace/leaktk-patterns/.git/hooks/pre-commit"
[INFO] installed hook: hook="git.pre-commit" path="/home/user/.config/git/template/hooks/pre-commit"
```

If a directory that you expected it to be installed into is missing from the output, you can run the command in
debug mode to see any directories that the command was unable to access or identify as a Git repository:

```sh
LEAKTK_LOGGER_LEVEL=DEBUG leaktk install hook git.pre-commit --force --user-template-dir --path "${HOME}"
```
