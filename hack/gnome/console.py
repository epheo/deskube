#!/usr/bin/env python3
"""Runs a command on a VM's serial console and prints its output.

    console.py <vm> <namespace> <user> <password> '<command>'

For guests with no network. Drives `virtctl console` through a pty with
a tiny expect loop, so no extra dependency is needed.
"""
import os, pty, select, sys, time

def main():
    vm, ns, user, password, cmd = sys.argv[1:6]
    marker = "__DESKUBE_DONE__"
    pid, fd = pty.fork()
    if pid == 0:
        os.execvp("virtctl", ["virtctl", "console", vm, "-n", ns])
    buf = b""

    def expect(patterns, timeout):
        nonlocal buf
        end = time.time() + timeout
        while time.time() < end:
            r, _, _ = select.select([fd], [], [], 0.5)
            if fd in r:
                try:
                    chunk = os.read(fd, 4096)
                except OSError:
                    return None
                buf += chunk
                for p in patterns:
                    if p.encode() in buf:
                        return p
        return None

    def send(s):
        os.write(fd, s.encode())

    time.sleep(2)
    send("\n")
    if expect(["login:"], 30) is None:
        send("\n")
        if expect(["login:"], 30) is None:
            print("no login prompt", file=sys.stderr); sys.exit(1)
    send(user + "\n")
    if expect(["Password:"], 15) is None:
        print("no password prompt", file=sys.stderr); sys.exit(1)
    send(password + "\n")
    if expect(["$ ", "# "], 20) is None:
        print("login failed", file=sys.stderr); sys.exit(1)
    buf = b""
    send(cmd + "; echo " + marker + "\n")
    expect([marker + "\r\n", marker + "\n"], 120)
    # drop the echoed command line and the marker
    out = buf.decode(errors="replace")
    out = out.split("\n", 1)[1] if "\n" in out else out
    out = out.replace(marker, "")
    print(out.replace("\r", ""), end="")
    send("exit\n")
    time.sleep(1)
    send("\x1d")  # Ctrl+] leaves virtctl console
    time.sleep(0.5)
    try:
        os.kill(pid, 9)
    except OSError:
        pass

main()
