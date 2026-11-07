#!/usr/bin/env bash
# run.sh — host-side wrapper for the native-engine docker self-test
# battery. Builds the image from the worktree root (the .dockerignore
# whitelist keeps the context tiny) and runs the battery INSIDE docker.
#
# Operator use:   ./docker/selftest/run.sh
# Agent use:      vh-agent-harness exec bash docker/selftest/run.sh
#                 (equivalently the two docker commands below, run
#                 through `vh-agent-harness exec docker ...`)
#
# WHY --security-opt seccomp=unconfined: Docker's DEFAULT seccomp
# profile blocks the landlock syscalls (landlock_create_ruleset,
# landlock_add_rule, landlock_restrict_self), so the engine's kernel
# confinement backend cannot arm itself under the default profile. The
# flag lifts ONLY Docker's profile — our OWN trampoline (the engine's
# execsandbox: Landlock + seccomp + no_new_privs) then enforces the
# confinement for sandboxed run_shell calls. This is exactly the
# deployment posture the sandbox scenarios test: sandbox_denial asserts
# a read-only run_shell write is kernel-denied INSIDE this container.
set -euo pipefail

cd "$(dirname "$0")/../.." # worktree root = docker build context

docker build -f docker/selftest/Dockerfile -t vh-selftest .
exec docker run --rm --security-opt seccomp=unconfined vh-selftest
