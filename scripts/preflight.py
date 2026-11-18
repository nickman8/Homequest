#!/usr/bin/env python3
"""Read private config. Use standard curl; keys/Telegram URLs stay off argv/logs."""
import json, os, subprocess, sys
s = dict(os.environ)
if len(sys.argv) > 1:
    with open(sys.argv[1]) as f:
        for key, value in json.load(f).items():
            s.setdefault(key, value)
checks = []
if s.get('TELEGRAM_BOT_TOKEN'):
    for method in ('getMe', 'getWebhookInfo'):
        checks.append((method, 'https://api.telegram.org/bot' + s['TELEGRAM_BOT_TOKEN'] + '/' + method, {}))
if s.get('NICKLLM_API_KEY'):
    checks.append(('models', s.get('NICKLLM_BASE_URL', 'https://vyceai.com/v1').rstrip('/') + '/models', {'Authorization': 'Bearer ' + s['NICKLLM_API_KEY']}))
if not checks:
    sys.exit('Credentials not configured')
for name, url, headers in checks:
    config = 'url = ' + json.dumps(url) + '\n'
    for key, value in headers.items():
        config += 'header = ' + json.dumps(key + ': ' + value) + '\n'
    try:
        result = subprocess.run(['curl', '--config', '-', '--silent', '--show-error', '--max-time', '25', '--write-out', '\n%{http_code}'], input=config, capture_output=True, text=True, timeout=30)
        if result.returncode:
            raise ValueError('transport')
        body, status = result.stdout.rsplit('\n', 1)
        out = {'check': name, 'status': int(status)}
        if status == '200':
            data = json.loads(body)
            if name == 'getMe':
                out['username'] = data.get('result', {}).get('username')
            elif name == 'getWebhookInfo':
                out['webhook_present'] = bool(data.get('result', {}).get('url'))
            else:
                ids = [m.get('id') for m in data.get('data', [])]
                out['model_count'] = len(ids)
                out['configured_model_present'] = s.get('NICKLLM_MODEL') in ids
                out['deepseek_model_ids'] = [m for m in ids if isinstance(m, str) and 'deepseek' in m.lower()]
        print(json.dumps(out))
    except Exception:
        print(json.dumps({'check': name, 'error': 'transport_or_response_error'}))
