#!/usr/bin/env bash
# write-all.sh — sequential spec writers (deepseek-flash), one per section.
# Skips sections already written. Logs to writer-log-<section>.txt.
set -u
SPEC=/media/ajaxiis/ext3/TNS65/fine/02-spec
cd "$SPEC" || exit 1
SECTIONS="00-terms 01-data 02-machine-birth 03-live-turn 04-workshop 05-chapter-book 06-frames 07-ui 08-open 09-acceptance"
for s in $SECTIONS; do
  out="$SPEC/$s.md"
  if [ -s "$out" ]; then
    echo "SKIP $s (exists)"
    continue
  fi
  sed "s/{SECTION}/$s/g" writer-prompt.tpl.md > "prompt-$s.md"
  echo "RUN  $s $(date +%H:%M:%S)"
  hermes chat -Q -t file --provider deepseek -m deepseek-flash --source spec-writer --yolo \
    -q "$(cat "prompt-$s.md")" > "writer-log-$s.txt" 2>&1
  rc=$?
  if [ -s "$out" ] && head -1 "$out" | grep -q '^#'; then
    lines=$(wc -l < "$out")
    echo "OK   $s (rc=$rc, $lines lines) $(date +%H:%M:%S)"
  else
    echo "FAIL $s (rc=$rc) — see writer-log-$s.txt"
  fi
done
echo "ALL-DONE $(date +%H:%M:%S)"
