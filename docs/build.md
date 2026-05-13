# Building LeakTK from source

This covers how to build LeakTK from source in more detail.

## Dependencies

If you have the `CGO_ENABLED` environment variable set to `0`, then
a standard go build should work out of the box.

You can check it with this command:

```sh
go env | grep '^CGO_ENABLED'
```

If it's set to `1` (i.e. it's enabled), you can disable it in your current
terminal session like this:

```sh
export CGO_ENABLED='0'
```

If you want to make it permanent for your go environment, you can run:

```sh
go env -w CGO_ENABLED='0'
```

If you can't or don't want to build LeakTK with cgo disabled, you will need
to install these build dependencies:

**Fedora/RHEL:**

```sh
sudo dnf install btrfs-progs-devel
```

**Ubuntu/Debian:**

```sh
apt-get install libbtrfs-dev
```

## Running the Build

After the dependencies are installed, run this command from the root of the
project directory:

```
go build
```

This should generate a `leaktk` binary in the same directory.

> **🗒️ NOTE: If you want version info set**
>
> LeakTK's version info is set via special build flags. If you want to set
> the version info, see the `build` target in the [Makefile](../Makefile).
