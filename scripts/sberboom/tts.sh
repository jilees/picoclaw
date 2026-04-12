#!/bin/bash
# tts.sh — Test TTS script for SberBoom channel (VPS debugging).
#
# Receives the response text as $1 and delivers it via an available channel.
# Replace this script with the real SberBoom TTS command when deploying on the device.
#
# Usage: ./tts.sh "Text to speak"

set -euo pipefail

TEXT="$1"

if [ -z "$TEXT" ]; then
  echo "tts.sh: empty text, nothing to do" >&2
  exit 0
fi

# Send to Telegram so we can observe the response during VPS testing.
# Adjust --target to your Telegram user/chat ID.
picoclaw message send --channel telegram --target 308837107 --message "$TEXT"
