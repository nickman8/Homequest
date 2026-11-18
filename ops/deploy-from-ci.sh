#!/usr/bin/env bash
set -euo pipefail
: "${DEPLOY_HOST:?}" "${DEPLOY_KEY:?}" "${DEPLOY_KNOWN_HOSTS:?}" "${GITHUB_SHA:?}" "${RUNNER_TEMP:?}"
[[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || exit 2
[[ "$DEPLOY_HOST" =~ ^[A-Za-z0-9][A-Za-z0-9.-]+$ ]] || exit 2
umask 077
key="$RUNNER_TEMP/homequest-deploy-key"
known="$RUNNER_TEMP/homequest-known-hosts"
trap 'rm -f -- "$key" "$known"' EXIT
printf '%s\n' "$DEPLOY_KEY" > "$key"
printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > "$known"
opts=(-i "$key" -o BatchMode=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile="$known" -o ConnectTimeout=15)
remote="homequest-deploy@$DEPLOY_HOST"
ssh "${opts[@]}" "$remote" "sudo -n /usr/local/sbin/homequest-deploy --check-upload"
scp "${opts[@]}" "dist/$GITHUB_SHA.tar" "$remote:/var/lib/homequest-deploy/incoming/$GITHUB_SHA.tar.part"
ssh "${opts[@]}" "$remote" "mv /var/lib/homequest-deploy/incoming/$GITHUB_SHA.tar.part /var/lib/homequest-deploy/incoming/$GITHUB_SHA.tar && sudo -n /usr/local/sbin/homequest-deploy $GITHUB_SHA"
