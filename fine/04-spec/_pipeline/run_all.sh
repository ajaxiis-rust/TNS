#!/usr/bin/env bash
# Batch: 3 lanes over remaining sections (00 = smoke, done before launch).
set -u
W=/media/ajaxiis/ext3/TNS65/fine/04-spec/_pipeline
cd "$W"
lane() { for nn in "$@"; do bash run_one.sh "$nn"; done; }
lane 01 04 07 &
lane 02 05 08 &
lane 03 06 09 &
wait
python3 completeness.py
RC=$?
echo "[$(date +%H:%M:%S)] BATCH-FINISHED completeness_exit=$RC" >> "$W/progress.log"
