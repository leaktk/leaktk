# Installing POSIX Hooks

This guide walks you through the process of installing LeakTK hooks for hooking
into parts [POSIX systems](https://en.wikipedia.org/wiki/POSIX).

| Hook Name | Audience | Purpose |
| --------- | -------- | ------- |
| posix.stdio | CI/CD Engineers, Automation Developers, System Administrators | Intercept and redact secrets from stdio streams |

> :page_facing_up: **NOTE:** The posix.stdio hook is not ideal for interactive
> terminals. It does work in some cases, but it is mainly meant for inclusion
> in scripts that work with secrets.

## POSIX Stdio Hook Install Options

The `posix.stdio` hook redirects a shell's standard output (`stdout`) and
standard error (`stderr`) streams through `leaktk redact` to mask secrets.

The hook can be added directly to the top of scripts (option 1) or added to
your shell's rc file (option 2) to enable it across multiple shell executions.

> **⚠️  WARNING: Gotcha with install on some CI/CD systems**
>
> Some CI/CD systems run steps in ephemeral environments or don't automatically
> source the shell's rc file. This could cause the hook install to not take
> effect.
>
> You can test that the hook is properly loaded by adding a test step that runs
> something like this to confirm it is redacting secrets:
>
> ```sh
> echo 'secret="2MnzSSNo2zOCjKXCAKPueYcjNXS6CDyAl8L6yT/gWW8="'
> ```

### Option 1: In Individual Scripts (Targeted Protection)

In this approach, add the hook initialization immediately after the script's
shebang to automatically redact sensitive information from the scripts stdout
and stderr.

Hook initialization command:

```sh
eval "$(leaktk hook posix.stdio)"
```

Example in a script:

```sh
#!/bin/sh
eval "$(leaktk hook posix.stdio)"
set -xeu

SECRET_API_KEY="2MnzSSNo2zOCjKXCAKPueYcjNXS6CDyAl8L6yT/gWW8="
# All subsequent command output and errors are now automatically redacted
echo "Deploying application with API_KEY='${SECRET_API_KEY}'..."
# ...
```

This can also catch sensitive command arguments printed out by `set -x`.

### Option 2: System-Wide in CI/CD Runners (Broad Enforcement)

The hook can also be installed via:

```sh
# Example showing installing into ~/.bashrc
leaktk install hook posix.stdio --bashrc
```

This is for use cases where you cannot easily modify every individual script or
third-party step executed by a build system.

By executing the installer during your CI/CD runner startup or container image
build process, `leaktk` automatically targets shell configuration files (such
as `~/.zshrc` via the `--zshrc` flag or `~/.bashrc` via the `--bashrc` flag):

### Known Issues

This is a new feature and we are testing it out with different CI/CD platforms
still.

We've noticed the default `eval "$(leaktk hook posix.stdio)"` command causing
issues with GitLab CI.

This seems to work as a workaround while we look into that issue:

```yaml
some_job:
  script: |
      exec {_leaktk_hook_posix_stdio_stdout_fd}>&1
      exec {_leaktk_hook_posix_stdio_stderr_fd}>&2
      exec 1> >(leaktk redact --kind Stdio >&${_leaktk_hook_posix_stdio_stdout_fd})
      exec 2> >(leaktk redact --kind Stdio >&${_leaktk_hook_posix_stdio_stderr_fd})
      unset _leaktk_hook_posix_stdio_stdout_fd
      unset _leaktk_hook_posix_stdio_stderr_fd

      # the rest of the script goes here...
```
