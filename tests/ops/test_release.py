import os, hashlib, importlib.util, io, json, pathlib, tarfile, tempfile, unittest
spec = importlib.util.spec_from_file_location('hq_deploy', pathlib.Path(__file__).parents[2] / 'ops/deploy.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
SHA = 'a' * 40
class ReleaseTests(unittest.TestCase):
    def bundle(self, root, corrupt=False, evil=None):
        files = {name: b'fixture' for name in module.FILES}
        manifest = {'commit': SHA, 'files': {name: hashlib.sha256(data).hexdigest() for name, data in files.items()}}
        if corrupt: files['bin/homequest'] = b'changed'
        archive = root / 'release.tar'
        with tarfile.open(archive, 'w') as tf:
            for name, data in {**files, 'manifest.json': json.dumps(manifest).encode()}.items():
                info = tarfile.TarInfo(name); info.size = len(data); tf.addfile(info, io.BytesIO(data))
            if evil:
                info = tarfile.TarInfo(evil); info.type = tarfile.SYMTYPE; info.linkname = '/etc/passwd'; tf.addfile(info)
        return archive
    def test_valid_release_and_commit_binding(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d); archive = self.bundle(root)
            manifest = module.unpack(archive, root/'release', SHA)
            self.assertEqual(manifest['commit'], SHA)
            with self.assertRaises(ValueError): module.unpack(archive, root/'wrong', 'b'*40)
    def test_checksum_and_archive_escape_rejected(self):
        for options in [{'corrupt':True}, {'evil':'../../escape'}, {'evil':'bin/link'}]:
            with self.subTest(options=options), tempfile.TemporaryDirectory() as d:
                root=pathlib.Path(d)
                with self.assertRaises(ValueError): module.unpack(self.bundle(root, **options), root/'release', SHA)
    def test_runtime_directories_traversable_under_root_umask(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d); archive=self.bundle(root)
            old=os.umask(0o077)
            try: module.unpack(archive, root/'release', SHA)
            finally: os.umask(old)
            for name in ['', 'bin', 'widgets', 'scripts']:
                self.assertEqual((root/'release'/name).stat().st_mode & 0o777, 0o755)
    def test_invalid_commit_rejected(self):
        for value in ['main', '../etc', SHA+';id', 'A'*40]:
            with self.assertRaises(ValueError): module.check_commit(value)
if __name__ == '__main__': unittest.main()
