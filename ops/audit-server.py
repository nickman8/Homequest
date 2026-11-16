#!/usr/bin/env python3
"""Read-only preinstallation evidence. Does not read service secrets or change state."""
import datetime, json, os, pathlib, platform, shutil, socket, subprocess

def command(argv):
    try:
        r=subprocess.run(argv, capture_output=True, text=True, timeout=15)
        return {'status':r.returncode, 'output':r.stdout[:20000]}
    except (OSError, subprocess.TimeoutExpired): return {'status':'unavailable'}

memory={}
for line in pathlib.Path('/proc/meminfo').read_text().splitlines():
    key,value=line.split(':',1)
    if key in ('MemTotal','MemAvailable','SwapTotal','SwapFree'): memory[key]=int(value.split()[0])*1024
free=shutil.disk_usage('/var/lib').free
sock=socket.socket()
try: sock.bind(('127.0.0.1',2022)); port_free=True
except OSError: port_free=False
finally: sock.close()
paths=['/opt/homequest','/etc/homequest','/var/lib/homequest','/var/lib/homequest-deploy',
       '/etc/systemd/system/homequest.service','/usr/local/sbin/homequest-deploy',
       '/etc/sudoers.d/homequest-deploy']
blockers=[]
if platform.system()!='Linux' or platform.machine()!='x86_64': blockers.append('Requires reviewed Linux amd64 build')
if not pathlib.Path('/run/systemd/system').is_dir(): blockers.append('No running systemd')
if (os.cpu_count() or 0)<2: blockers.append('Less than 2 logical CPUs')
if memory.get('MemAvailable',0)<3*1024**3: blockers.append('Less than 3 GiB currently available memory; STT co-location needs review')
if free<5*1024**3: blockers.append('Less than 5 GiB free disk for runtime, STT, releases and backups')
if not port_free: blockers.append('Loopback port 2022 occupied; identify owner before deployment')
existing=[p for p in paths if os.path.lexists(p)]
if existing: blockers.append('HomeQuest target paths already exist; inspect before any installation')
result={'checked_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'os':platform.system(),'architecture':platform.machine(),'python':platform.python_version(),
        'cpus':os.cpu_count(),'load':os.getloadavg(),'memory_bytes':memory,'free_disk_bytes':free,
        'port_2022_free':port_free,'existing_paths':existing,'blockers':blockers,
        'running_services':command(['systemctl','list-units','--type=service','--state=running','--no-pager','--no-legend']),
        'listening_ports':command(['ss','-lnt']),
        'containers':command(['docker','ps','--format','{{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}']),
        'container_load':command(['docker','stats','--no-stream','--format','{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}']),
        'process_memory':command(['ps','-eo','comm,pcpu,pmem,rss','--sort=-rss']),
        'review_required':'Identify LiteLLM/database/reverse proxy, baseline health and load; verify venv support. Empty blockers are not approval.'}
print(json.dumps(result,ensure_ascii=False,indent=2))
