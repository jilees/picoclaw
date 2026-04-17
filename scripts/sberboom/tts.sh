#!/bin/sh
# tts.sh — Speak text via SberBoom native TTS.
# Usage: ./tts.sh "Text to speak"

set -u

BOX_BIN="/vendor/staros/box"
STAR_JSON="/vendor/staros/star.json"
REQUEST_ID="picoclaw"

TEXT="$1"

if [ -z "$TEXT" ]; then
  echo "tts.sh: empty text, nothing to do" >&2
  exit 0
fi

if [ ! -x "$BOX_BIN" ]; then
  echo "tts.sh: $BOX_BIN not found or not executable" >&2
  exit 1
fi

if [ ! -f "$STAR_JSON" ]; then
  echo "tts.sh: $STAR_JSON not found" >&2
  exit 1
fi

# Escape backslashes and double quotes for the protobuf text field.
text_esc="$(printf '%s' "$TEXT" | sed 's/\\/\\\\/g; s/"/\\"/g')"
req_esc="$(printf '%s' "$REQUEST_ID" | sed 's/\\/\\\\/g; s/"/\\"/g')"

proto="process_star_command{assistant_text_to_speech{base_command{source{local{request_id:\"${req_esc}\"}}}text_to_pronounce:\"${text_esc}\"}}"

exec "$BOX_BIN" --app client -f "$STAR_JSON" -v 1 "$proto"
