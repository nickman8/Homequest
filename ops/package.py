#!/usr/bin/env python3
"""Package only tested runtime files. No Git metadata, config, DB or secrets."""
import hashlib, io, json, pathlib, subprocess, tarfile
from deploy import FILES, check_commit
commit = check_commit(subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip())
files = {name: pathlib.Path(name).read_bytes() for name in FILES}
manifest = {'commit': commit, 'files': {name: hashlib.sha256(data).hexdigest() for name, data in files.items()}}
pathlib.Path('dist').mkdir(exist_ok=True)
with tarfile.open('dist/' + commit + '.tar', 'w') as tf:
    for name, data in {**files, 'manifest.json': json.dumps(manifest).encode()}.items():
        info = tarfile.TarInfo(name); info.size = len(data); info.mode = 0o644
        tf.addfile(info, io.BytesIO(data))
print('Packaged release ' + commit)
