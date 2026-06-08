#!/usr/bin/env bash
# Synthesize a 2s tick-tock loop and upload to MinIO as a proper object.
set -e
OUT=/tmp/quiz-tick.mp3
ffmpeg -y -f lavfi -i "sine=frequency=1800:duration=0.04" -f lavfi -i "sine=frequency=1200:duration=0.04" \
  -filter_complex "[0]apad=pad_dur=0.46[a];[1]apad=pad_dur=0.46[b];[a][b]concat=n=2:v=0:a=1,aloop=loop=1:size=88200,atrim=0:2,volume=0.6[out]" \
  -map "[out]" -ar 44100 -ac 2 "$OUT"
docker cp "$OUT" webcomics-minio:/tmp/quiz-tick.mp3
docker exec webcomics-minio sh -c "mc alias set local http://localhost:9000 minioadmin minioadmin >/dev/null 2>&1; mc cp /tmp/quiz-tick.mp3 local/webcomics/library/sfx/quiz-tick.mp3"
echo "uploaded library/sfx/quiz-tick.mp3 as object"
