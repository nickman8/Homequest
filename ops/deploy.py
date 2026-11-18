#!/usr/bin/python3
"""Root-owned, narrowly scoped activator. Install only after the server audit.
No package manager, firewall, proxy, Docker, LiteLLM or external service changes.
"""
import fcntl
import hashlib
import json
import os
import pathlib
import re
import shutil
import sqlite3
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request

FILES = ('bin/homequest', 'bin/homequest-migrate', 'widgets/family.html',
         'scripts/transcribe.py', 'scripts/setup-stt.py', 'requirements-stt.lock')
MAX_BYTES = 150 * 1024 * 1024
BASE = pathlib.Path('/opt/homequest')
DATA = pathlib.Path('/var/lib/homequest')
STATE = pathlib.Path('/var/lib/homequest-deploy')
ETC = pathlib.Path('/etc/homequest')
SERVICE = 'homequest.service'


def check_commit(value):
    if not re.fullmatch('[0-9a-f]{40}', value):
        raise ValueError('Expected a full lowercase commit SHA')
    return value


def unpack(archive, destination, commit):
    check_commit(commit)
    destination.mkdir(mode=0o755)
    destination.chmod(0o755)
    with tarfile.open(archive, 'r:') as tf:
        members = tf.getmembers()
        names = [m.name for m in members]
        if set(names) != set(FILES) | {'manifest.json'} or len(names) != len(set(names)):
            raise ValueError('Unexpected or duplicate archive paths')
        if any(not m.isfile() or m.size < 0 for m in members) or sum(m.size for m in members) > MAX_BYTES:
            raise ValueError('Unsafe archive members or size')
        if tf.getmember('manifest.json').size > 4096:
            raise ValueError('Manifest too large')
        manifest = json.load(tf.extractfile('manifest.json'))
        if manifest.get('commit') != commit or set(manifest.get('files', {})) != set(FILES):
            raise ValueError('Manifest does not match requested release')
        for name in FILES:
            data = tf.extractfile(name).read()
            if hashlib.sha256(data).hexdigest() != manifest['files'][name]:
                raise ValueError('Release checksum mismatch')
            path = destination / name
            path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
            path.parent.chmod(0o755)
            path.write_bytes(data)
            path.chmod(0o755 if name.startswith('bin/') else 0o644)
        (destination / 'manifest.json').write_text(json.dumps(manifest))
    return manifest


def backup(source, destination):
    # Works for migration rollback only while the service is stopped.
    if destination.exists():
        raise ValueError('Backup destination already exists')
    with sqlite3.connect('file:' + str(source) + '?mode=ro', uri=True) as a, sqlite3.connect(destination) as b:
        a.backup(b)
    destination.chmod(0o600)


def systemctl(action):
    subprocess.run(['/usr/bin/systemctl', action, SERVICE], check=True,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=45)


def point_to(release):
    link = BASE / '.current-next'
    link.unlink(missing_ok=True)
    link.symlink_to(release)
    link.replace(BASE / 'current')


def ready():
    # Never send local health through an inherited internet proxy.
    if subprocess.run(['/usr/bin/systemctl', 'is-active', '--quiet', SERVICE],
                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5).returncode:
        return False
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with client.open('http://127.0.0.1:2022/health', timeout=2) as response:
            data = json.load(response)
        return all(data.get(k) == v for k, v in
                   {'database': 'ready', 'scheduler': 'ready', 'telegram': 'polling'}.items())
    except Exception:
        return False


def activate(release, migrate, healthy, control=systemctl):
    """Failures before startup can restore DB; after startup retain new facts."""
    previous = (BASE / 'current').resolve() if (BASE / 'current').exists() else None
    db = DATA / 'donetick.db'
    had_db = db.exists()
    backups = STATE / 'backups'
    backups.mkdir(mode=0o700, exist_ok=True)
    saved = backups / (str(time.time_ns()) + '.db')
    database_bytes = sum(p.stat().st_size for p in DATA.glob('donetick.db*') if p.is_file())
    if shutil.disk_usage(STATE).free < database_bytes * 2 + 2 * 1024**3:
        raise RuntimeError('Insufficient disk reserve; service left unchanged')
    control('stop')
    if had_db:
        backup(db, saved)
    try:
        migrate(release)
    except Exception:
        # Candidate has never polled, so no newly accepted facts can be lost.
        for suffix in ('', '-wal', '-shm', '-journal'):
            pathlib.Path(str(db) + suffix).unlink(missing_ok=True)
        if had_db:
            shutil.copyfile(saved, db)
            db.chmod(0o600)
            if os.geteuid() == 0:
                shutil.chown(db, user='homequest', group='homequest')
        if previous:
            control('start')
        raise RuntimeError('Migration failed; pre-start database restored') from None
    point_to(release)
    try:
        control('start')
        for _ in range(30):
            if healthy():
                return
            time.sleep(1)
    except Exception:
        pass
    control('stop')
    # Do not restore the database: the candidate may already have handled updates.
    # Keep the failed candidate and snapshot for an explicit, data-aware recovery.
    raise RuntimeError('Readiness failed; HomeQuest stopped, database retained; operator recovery required')


def prune(current):
    # Only our own immutable releases and named snapshots, after successful health.
    releases = sorted((p for p in (BASE/'releases').iterdir()
                       if p.is_dir() and not p.is_symlink() and re.fullmatch('[0-9a-f]{40}',p.name)),
                      key=lambda p:p.stat().st_mtime_ns, reverse=True)
    keep = {current, *releases[:3]}
    for p in releases:
        if p not in keep: shutil.rmtree(p)
    snapshots = sorted((p for p in (STATE/'backups').glob('*.db')
                        if p.is_file() and not p.is_symlink() and re.fullmatch('[0-9]+.db',p.name)),
                       key=lambda p:int(p.stem), reverse=True)
    for p in snapshots[7:]: p.unlink()


def migrate(release):
    subprocess.run(['/usr/sbin/runuser', '-u', 'homequest', '--', '/usr/bin/env',
                    'DATA_DIR=' + str(DATA), str(release / 'bin/homequest-migrate')],
                   check=True, timeout=90, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def ensure_capacity():
    memory = dict(line.split(':', 1) for line in pathlib.Path('/proc/meminfo').read_text().splitlines())
    if int(memory['MemAvailable'].split()[0]) * 1024 < 2 * 1024**3:
        raise ValueError('Less than 2 GiB available memory; no deployment started')
    database_bytes = sum(p.stat().st_size for p in DATA.glob('donetick.db*') if p.is_file())
    if shutil.disk_usage(STATE).free < 2 * MAX_BYTES + 2 * database_bytes + 2 * 1024**3:
        raise ValueError('Insufficient disk reserve for upload, release and database backup')


def deploy(commit):
    check_commit(commit)
    if os.geteuid() != 0:
        raise ValueError('Run the installed root-owned activator through the dedicated sudo rule')
    # These markers are created by the operator only after audit and poller handoff.
    for name in ('audit-approved', 'allow-polling', 'secrets.json', 'stt-lock.sha256'):
        path = ETC / name
        st = path.lstat()
        if not stat.S_ISREG(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o022:
            raise ValueError('Deployment prerequisite missing or insecure')
    ensure_capacity()
    os.umask(0o077)
    with open(ETC / 'deploy.lock', 'a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        releases = BASE / 'releases'
        releases.mkdir(mode=0o755, parents=True, exist_ok=True)
        releases.chmod(0o755)
        # Root-owned staging closes races with the unprivileged upload account.
        with tempfile.TemporaryDirectory(dir=releases, prefix='.staging-') as tmp:
            tmp = pathlib.Path(tmp)
            fd = os.open(STATE / 'incoming' / (commit + '.tar'), os.O_RDONLY | os.O_NOFOLLOW)
            with os.fdopen(fd, 'rb') as src, open(tmp / 'archive.tar', 'wb') as dst:
                if not stat.S_ISREG(os.fstat(src.fileno()).st_mode):
                    raise ValueError('Upload must be a regular file')
                remaining = MAX_BYTES + 1024 * 1024
                while chunk := src.read(min(1024 * 1024, remaining + 1)):
                    remaining -= len(chunk)
                    if remaining < 0:
                        raise ValueError('Upload too large')
                    dst.write(chunk)
            candidate = tmp / 'release'
            manifest = unpack(tmp / 'archive.tar', candidate, commit)
            if manifest['files']['requirements-stt.lock'] != (ETC / 'stt-lock.sha256').read_text().strip():
                raise ValueError('STT lock changed: isolated venv update and audit required')
            release = releases / commit
            if release.exists():
                if (release / 'manifest.json').read_text() != (candidate / 'manifest.json').read_text():
                    raise ValueError('Existing release differs')
            else:
                candidate.replace(release)
            if (BASE / 'current').exists() and (BASE / 'current').resolve() == release and ready():
                print('Release already healthy')
                return
            activate(release, migrate, ready)
            prune(release)
            (STATE / 'incoming' / (commit + '.tar')).unlink(missing_ok=True)
            print('HomeQuest release healthy: ' + commit)


if __name__ == '__main__':
    try:
        if len(sys.argv) != 2:
            raise ValueError('Usage: homequest-deploy FULL_COMMIT_SHA')
        if sys.argv[1] == '--check-upload':
            ensure_capacity()
            print('Upload capacity available')
        else:
            deploy(sys.argv[1])
    except (ValueError, RuntimeError) as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
    except Exception:
        print('Deployment failed; inspect service status and local deployment prerequisites', file=sys.stderr)
        sys.exit(1)
