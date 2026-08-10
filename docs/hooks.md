# Hooks

LeakTK-Hooks allows you to integrate secrets scanning into existing tools.

## Git Hooks

LeakTK supports the following [Git hooks](https://git-scm.com/docs/githooks):

| Hook Name       | Audience                  | Purpose                                       |
| --------------- | ------------------------- | --------------------------------------------- |
| git.pre-commit  | Developers                | Block creating new commits containing secrets |
| git.pre-receive | Git Server Administrators | Block Git pushes containing secrets           |

See the hook specific [install guide](install_git_hooks.md) to get started.

## Standard Input/Output Hook

LeakTK supports the following standard stream hook:

| Hook Name | Audience | Purpose |
| --------- | -------- | ------- |
| posix.stdio | CI/CD Engineers, Automation Developers, System Administrators | Intercept and redact secrets from stdio streams |

See the hook specific [install guide](install_posix_hooks.md) to get started.

## Planned

These are the hooks we want to implement next:

- Claude Code Hooks
- Cursor Hooks
- Gemini CLI Hooks
- Google Cloud Storage Hooks
- AWS S3 Storage Hooks

For more info or to request a hook type not covered here, leave a comment on:
https://github.com/leaktk/leaktk/issues/238
