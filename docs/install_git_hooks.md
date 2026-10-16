# Installing Git Hooks

This guide walks you through the process of installing
LeakTK's supported [Git hooks](https://git-scm.com/docs/githooks).

| Hook Name       | Audience                  | Purpose                                       |
| --------------- | ------------------------- | --------------------------------------------- |
| git.pre-commit  | Developers                | Block creating new commits containing secrets |
| git.pre-receive | Git Server Administrators | Block Git pushes containing secrets           |

## Prerequisites

This guide requires that you have the `leaktk` command installed. If you do not
already have it installed, see the main [installation guide](install.md).

## Steps

> **🗒️ NOTE: Alternate install methods are at the end**
>
> Some teams use Git hook managers like the one provided by pre-commit.com.
> There is a section at the end for those.

For this guide we're going to use the Git pre-commit hook, but the process is
similar for the other hooks.

1. Choose the hook you want to install (in our case it's `git.pre-commit`).
1. Decide where you want to install it (this example will enable it for all of
   the user's existing and new repositories without replacing existing hooks).
1. Run the install command with the proper flags set:
   ```sh
   leaktk install hook git.pre-commit --user-config
   ```

You should see output similar to this:

```
[INFO] git hook installed: event="pre-commit" command="leaktk hook git.pre-commit" scope="global"
```

If you get an error like the one below, follow the steps in
[Git Before v2.54.0](#git-before-v2540) instead.

```
[CRITICAL] could not install git hook: git version does not support config-based hooks: min_version="2.54.0" current_version="2.52.0" hookname="git.pre-commit"
```

## Alternate Install Methods

We won't be able to list every method here, but we plan to add more here as
we get questions about different methods.

### Git Before v2.54.0

> **🗒️ NOTE: Only allows one hook in a repositories per Git event**
>
> Before v2.54.0, Git did not support
> [config-based git hooks](https://github.blog/open-source/git/highlights-from-git-2-54/#h-config-based-hooks),
> and without using a hook manager, could only run one hook per Git event
> (i.e. pre-commit, commit-msg, etc). Newer versions of Git support
> config-based hooks that allow multiple hooks to be installed at the repository, user,
> or system levels at the same time. Steps for using config-based hooks can be
> found [above](#steps).

This method covers how to install the hook for every repository under `"${HOME}"` and
for new clones for Git before v2.54.0.

1. Choose the hook you want to install (in our case it's `git.pre-commit`).
1. Decide where you want to install it (this example will assume all Git
   repositories under your home directory: `"${HOME}"`).
1. Decide if you want to replace any existing hooks (this example will assume
   yes so we'll add: `--force`)
1. Decide if you want it to be enabled for new repositories by default (this
   example assumes yes: `--user-template-dir`).
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

If a directory that you expected it to be installed into is missing from the
output, you can run the command in debug mode to see any directories that the
command was unable to access or identify as a Git repository:

```sh
LEAKTK_LOGGER_LEVEL=DEBUG leaktk install hook git.pre-commit --force --user-template-dir --path "${HOME}"
```

### .pre-commit-config.yaml

There is a popular git hook manager called [pre-commit](https://pre-commit.com)
that allows you to define hooks in the Git repository itself.

1. [Install pre-commit](https://pre-commit.com/#install)
2. Install the [build dependencies](build.md##dependencies)
3. Add a section like this to your repository's `.pre-commit-config.yaml`:
   ```yaml
   repos:
     - repo: https://github.com/leaktk/leaktk.git
       # We recommend pinning by commit. Run this command to get the latest
       # revision:
       #
       #   git ls-remote https://github.com/leaktk/leaktk.git refs/tags/v*
       #
       rev: 3a0b3201b0524d5f46dcfc8d8907b66cea7f7c8f
       hooks:
         - id: leaktk.git.pre-commit
   ```
4. Try out the hook
   ```sh
   pre-commit run --hook-stage pre-commit leaktk.git.pre-commit
   ```

### Custom

> **⚠️  WARNING: This hook only scans staged content**
>
> If you are running it in a CI/CD pipeline and you wish to scan existing
> Git history, run `leaktk scan` with the proper arguments instead. The
> pre-commit hooks only scans staged content. See `leaktk help scan` for
> more info.

If you have some custom case not covered here, the LeakTK git hooks can be
executed manually like this:

```sh
leaktk hook git.pre-commit
```

If you want to integrate LeakTK's pre-commit hook into an existing hook
manager, the hook manager needs to:

- Ensure that the leaktk command is installed
- Execute `leaktk hook git.pre-commit`
- Fail if the leaktk command exits with a non-zero exit status
