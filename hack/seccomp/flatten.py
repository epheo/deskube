#!/usr/bin/env python3
"""Flattens the containers-common seccomp profile for one situation.

That profile, podman's default, picks rules by the capabilities and the
architecture of the container. The Security Profiles Operator takes a
plain list. This resolves the conditions for a container on amd64 that
holds no capability, which is what a sandbox desktop is, and prints the
spec of a SeccompProfile.

    flatten.py [/usr/share/containers/seccomp.json] > internal/controller/sandbox_seccomp.json
"""
import json
import sys

SOURCE = sys.argv[1] if len(sys.argv) > 1 else "/usr/share/containers/seccomp.json"
ARCH = "amd64"
CAPS = set()

src = json.load(open(SOURCE))


def applies(rule):
    inc, exc = rule.get("includes") or {}, rule.get("excludes") or {}
    if inc.get("caps") and not set(inc["caps"]) <= CAPS:
        return False
    if inc.get("arches") and ARCH not in inc["arches"]:
        return False
    if exc.get("caps") and set(exc["caps"]) & CAPS:
        return False
    if exc.get("arches") and ARCH in exc["arches"]:
        return False
    return True


arches = []
for entry in src.get("archMap", []):
    if entry["architecture"] == "SCMP_ARCH_X86_64":
        # Steam's client is a 32 bit program: the sub architectures matter.
        arches = [entry["architecture"]] + entry.get("subArchitectures", [])

syscalls = []
for rule in src["syscalls"]:
    if not applies(rule):
        continue
    out = {"action": rule["action"], "names": sorted(rule["names"])}
    if rule.get("args"):
        out["args"] = [{k: a[k] for k in ("index", "value", "valueTwo", "op") if k in a} for a in rule["args"]]
    if rule.get("errnoRet") is not None:
        out["errnoRet"] = rule["errnoRet"]
    syscalls.append(out)

# The source both allows and refuses a few calls (setns) once its
# conditions are gone. Refusing wins: least privilege, and no runtime
# has to pick between two rules for one call.
refused = {n for r in syscalls if r["action"] != "SCMP_ACT_ALLOW" and "args" not in r for n in r["names"]}
for rule in syscalls:
    if rule["action"] == "SCMP_ACT_ALLOW" and "args" not in rule:
        rule["names"] = [n for n in rule["names"] if n not in refused]
syscalls = [r for r in syscalls if r["names"]]
syscalls = [r for i, r in enumerate(syscalls) if r not in syscalls[:i]]

spec = {"defaultAction": src["defaultAction"], "architectures": arches, "syscalls": syscalls}
json.dump(spec, sys.stdout, indent=1, sort_keys=True)
sys.stdout.write("\n")
