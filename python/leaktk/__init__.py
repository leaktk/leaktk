import os
import sys
import subprocess

pkg_dir = os.path.dirname(os.path.abspath(__file__))
bin_name = "leaktk.exe" if sys.platform == "win32" else "leaktk"
bin_path = os.path.join(pkg_dir, bin_name)


class LeakTK:
    path = bin_path
    check = False
    cwd = None
    encoding = None
    env = None
    errors = None
    input = None
    stderr = None
    stdin = None
    stdout = None
    text = None
    timeout = None
    universal_newlines = None

    def __init__(self, *args):
        self.args = args

    def run(self):
        return subprocess.run(
            [self.path, *self.args],
            check=self.check,
            cwd=self.cwd,
            encoding=self.encoding,
            errors=self.errors,
            input=self.input,
            shell=False,
            stderr=self.stderr,
            stdin=self.stdin,
            stdout=self.stdout,
            text=self.text,
            timeout=self.timeout,
            universal_newlines=self.universal_newlines,
        )


def main():
    return LeakTK(*sys.argv[1:]).run().returncode
