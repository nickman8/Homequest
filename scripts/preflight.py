#!/usr/bin/env python3
"""Read credentials outside Git; never print raw external errors or bodies."""
import json, os, sys, urllib.request, urllib.error
s=dict(os.environ)
if len(sys.argv)>1:
    with open(sys.argv[1]) as f:
        for k,v in json.load(f).items(): s.setdefault(k,v)
token=s.get('TELEGRAM_BOT_TOKEN'); key=s.get('NICKLLM_API_KEY')
checks=[]
if token:
    for m in ('getMe','getWebhookInfo'): checks.append((m,'https://api.telegram.org/bot'+token+'/'+m,{}))
if key: checks.append(('models',s.get('NICKLLM_BASE_URL','https://api-ai.nickproject.org/v1')+'/models',{'Authorization':'Bearer '+key}))
if not checks: sys.exit('Credentials not configured')
for name,url,headers in checks:
    try:
        with urllib.request.urlopen(urllib.request.Request(url,headers=headers),timeout=25) as r:
            d=json.load(r)
            out={'check':name,'status':r.status}
            if name=='getMe': out['username']=d.get('result',{}).get('username')
            elif name=='getWebhookInfo': out['webhook_present']=bool(d.get('result',{}).get('url'))
            else: out['model_ids']=[x.get('id') for x in d.get('data',[])]
            print(json.dumps(out))
    except urllib.error.HTTPError as e: print(json.dumps({'check':name,'status':e.code}))
    except Exception: print(json.dumps({'check':name,'error':'transport_or_response_error'}))
