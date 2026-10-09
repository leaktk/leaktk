import hashlib
import os
import platform
import requests
import shutil
import subprocess
import sys
import tarfile
import tempfile

from hatchling.builders.hooks.plugin.interface import BuildHookInterface

import re

release_re = re.compile(
    r"href=\"[^\s]*(leaktk-[^\/]+\.tar\.xz)\"[\s\S]*?(sha256:[0-9a-f]{64})",
    re.MULTILINE,
)


class PkgInfo:
    bin_name = None
    pkg_name = None
    pkg_version = None
    pkg_digest = None


def _getenv_bool(varname):
    value = os.getenv(varname)
    if value is None:
        return None

    return value.lower() in ("1", "true")


def _sys_type():
    p = platform.system().lower()
    if p not in {"linux", "windows", "darwin"}:
        return ""

    m = platform.machine()
    if m == "aarch64":
        m = "arm64"

    if m not in {"x86_64", "arm64"}:
        return ""

    return f"{p}-{m}"


def _leaktk_release(version):
    sys_type = _sys_type()

    p = PkgInfo()
    p.pkg_version = version.split("+", 1)[0]
    p.bin_name = "leaktk.exe" if sys_type.startswith("windows") else "leaktk"
    p.pkg_name = f"leaktk-{p.pkg_version}-{sys_type}.tar.xz"
    p.pkg_digest = None

    try:
        print("fetching release details")
        resp = requests.get(
            f"https://github.com/leaktk/leaktk/releases/expanded_assets/v{p.pkg_version}",
            timeout=10,
        )
        resp.raise_for_status()
        for file, digest in release_re.findall(resp.text):
            if file == p.pkg_name:
                p.pkg_digest = digest
                print(f"release found: {p.pkg_name}:{p.bin_name}@{p.pkg_digest}")
                break
    except Exception:
        pass

    if not p.pkg_digest:
        print(f"could not fetch release details for v{p.pkg_version}")

    return p


def _leaktk_download(p):
    download_url = (
        f"https://github.com/leaktk/leaktk/releases/download/v{p.pkg_version}/{p.pkg_name}"
    )
    dgst_algo, dgst_checksum = p.pkg_digest.split(":", 1)
    h = hashlib.new(dgst_algo)

    with tempfile.TemporaryFile() as pkg_file:
        print("downloading precompiled binary:", download_url, p.pkg_digest)
        resp = requests.get(download_url, stream=True)
        resp.raise_for_status()

        for chunk in resp.iter_content(chunk_size=4096):
            h.update(chunk)
            pkg_file.write(chunk)

        print("validating checksum:", p.pkg_digest)
        if h.hexdigest() != dgst_checksum:
            raise ValueError(
                f"invalid {dgst_algo} checksum: expected={dgst_checksum} actual={h.hexdigest()}"
            )

        print("extracting command:", p.bin_name)
        pkg_file.seek(0)
        with (
            tarfile.open(fileobj=pkg_file) as pkg_tar,
            open(p.bin_name, "wb") as dst_file,
        ):
            src_file = pkg_tar.extractfile(p.bin_name)
            shutil.copyfileobj(src_file, dst_file)


class LeakTKBinBuildHook(BuildHookInterface):
    PLUGIN_NAME = "leaktk-bin"

    def initialize(self, _: str, build_data: dict) -> None:
        if self.target_name != "wheel":
            return

        fetch_precompiled = _getenv_bool("LEAKTK_BUILD_FETCH_PRECOMPILED")
        if fetch_precompiled is None:
            config = self.metadata.config['tool']['hatch']['build']['hooks']['custom']
            fetch_precompiled = config.get('fetch_precompiled')

        pkg_info = _leaktk_release(self.metadata.version)
        if os.path.isfile(pkg_info.bin_name):
            os.unlink(pkg_info.bin_name)

        if fetch_precompiled and pkg_info.pkg_digest:
            print("attempting to download pre-compiled package")
            try:
                _leaktk_download(pkg_info)
            except Exception as e:
                print(f"failed to download pre-compiled package: {e}")
                if os.path.isfile(pkg_info.bin_name):
                    os.unlink(pkg_info.bin_name)

        if not os.path.isfile(pkg_info.bin_name):
            print("executing 'make build'...")
            try:
                subprocess.run(["make", "build"], check=True)
            except Exception as e:
                sys.exit(f"error running 'make build': {e}")

        build_data["force_include"][pkg_info.bin_name] = f"leaktk/{pkg_info.bin_name}"

    def clean(self, versions: list[str]) -> None:
        subprocess.run(["make", "clean"], check=True)
