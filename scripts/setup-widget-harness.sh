#!/usr/bin/env bash
set -euo pipefail
root="${WIDGET_SANDBOX:-$PWD/.tools/widget-sandbox}"
pin=1cde0068daba8437f53eac0b61c395d29b8f89e3
if [ ! -d "$root/.git" ]; then
  git clone https://github.com/TelegramMessenger/widget-sandbox "$root"
fi
if [ -n "$(git -C "$root" status --porcelain)" ]; then echo 'Harness checkout has changes' >&2; exit 1; fi
git -C "$root" checkout --detach "$pin"
(cd "$root" && corepack pnpm install --frozen-lockfile && corepack pnpm exec playwright install chromium)
