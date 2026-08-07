# Hooks

LeakTK-Hooks allows you to integrate secrets scanning into existing tools.

## Git Hooks

LeakTK supports the following [Git hooks](https://git-scm.com/docs/githooks):

| Hook Name       | Audience                  | Purpose                                       |
| --------------- | ------------------------- | --------------------------------------------- |
| git.pre-commit  | Developers                | Block creating new commits containing secrets |
| git.pre-receive | Git Server Administrators | Block Git pushes containing secrets           |

See the Git hook specific [install guide](install_git_hooks.md) to get started.

## Standard Input/Output Hook

LeakTK supports the following standard stream hook:

| Hook Name | Audience | Purpose |
| --------- | -------- | ------- |
| posix.stdio | CI/CD Engineers, Automation Developers, System Administrators | Intercept and redact secrets from stdio streams |

# Recommended Usage & Workflow
The `posix.stdio` hook redirects standard output (`stdout`) and standard error (`stderr`) streams through named FIFOs managed by background `leaktk redact` daemons.

# 1. In Individual Scripts (Targeted Protection)
To ensure execution logs from an automated script never leak sensitive credentials into build logs or storage buckets, add the hook initialization immediately after the script's shebang line:
```#!/bin/sh
#!/bin/sh
eval "$(leaktk hook posix.stdio)"

# All subsequent command output and errors are now automatically redacted
echo "Deploying application with API_KEY=${SECRET_API_KEY}..."
./deploy-service.sh
```
# 2. System-Wide in CI/CD Runners (Broad Enforcement)
The primary use case for `leaktk install hook posix.stdio` is environments where you cannot easily modify every individual script or third-party step executed by a build system.

By executing the installer during your CI/CD runner startup or container image build process, `leaktk` automatically targets shell configuration files (such as `~/.zshrc` or `~/.bashrc`), ensuring all spawned jobs and scripts on the runner are protected by default:
```
# Example: GitHub Actions Runner Setup Step
steps:
  - name: Install LeakTK POSIX Stdio Hook
    run: |
      leaktk install hook posix.stdio
      
  - name: Run Arbitrary Un-audited Build Scripts
    run: |
      # Inherits POSIX stdio redirection automatically
      ./run-legacy-build-pipeline.sh
```

```
```
## Planned

These are the hooks we want to implement next:

- Claude Code Hooks
- Cursor Hooks
- Gemini CLI Hooks
- Google Cloud Storage Hooks
- AWS S3 Storage Hooks

For more info or to request a hook type not covered here, leave a comment on:
https://github.com/leaktk/leaktk/issues/238
