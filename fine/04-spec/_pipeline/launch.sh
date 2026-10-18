#!/usr/bin/env bash
# Detached launcher (setsid survives gateway restarts).
cd "$(dirname "$0")"
setsid bash run_all.sh > run_all.log 2>&1 < /dev/null &
echo "run_all launched, wrapper pid $!"
