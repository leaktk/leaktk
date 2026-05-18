# Building LeakTK from source

This covers how to build LeakTK from source in more detail.

## Dependencies

LeakTK can be built with [cgo](https://go.dev/wiki/cgo) enabled or disabled.
We build our packages with cgo disabled, but this doc will cover the
dependencies needed to do it with either cgo enabled or disabled.

If you are using a tool that installs LeakTK automatically like pre-commit.com
style hooks, we recommend skipping to the section for installing dependencies
for when cgo is enabled.

Also `pre-commit>=3.0.0` will automatically [bootstrap go](https://pre-commit.com/#golang)
if it is not present—meaning the only LeakTK build dependancy for pre-commit.com is
the btrfs package for Linux users.

### Enabling/disabling cgo

You can check if cgo is enabled in your go env with this commmand.

```sh
go env | grep '^CGO_ENABLED'
```

If it's set to `1` it's enabled, and you can disable it in your current
terminal session like this:

```sh
export CGO_ENABLED='0'
```

See `go help env` if you want to make it permanent for your go environment, but
**disabling cgo for your whole go env can cause other builds to fail!**

### Dependencies with cgo enabled

If you can't or don't want to build LeakTK with cgo disabled, you will need
to install these build dependencies:

**Fedora/RHEL:**

```sh
dnf install make git golang btrfs-progs-devel
```

**Ubuntu/Debian:**

```sh
apt-get install make git golang libbtrfs-dev
```

**macOS**

```sh
xcode-select --install
```

And follow the steps [here](https://go.dev/doc/install) for installing go or
homebrew users can run `brew install go`.

### Dependencies with cgo disabled

**Fedora/RHEL:**

```sh
dnf install make golang git
```

**Ubuntu/Debian:**

```sh
apt-get install make golang git
```

**macOS**

```sh
xcode-select --install
```

And follow the steps [here](https://go.dev/doc/install) for installing go or
homebrew users can run `brew install go`.

## Running a build

After the dependencies are installed, run this command from the root directory
of the project to generate a `leaktk` binary in the same directory:

```sh
make
```
