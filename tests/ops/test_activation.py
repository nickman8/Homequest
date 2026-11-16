import importlib.util, pathlib, sqlite3, tempfile, unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('hq_activation',pathlib.Path(__file__).parents[2]/'ops/deploy.py')
m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
class ActivationTests(unittest.TestCase):
    def run_case(self, migration_fails=False, health_fails=False, disk_fails=False):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d); base=root/'opt'; data=root/'data'; state=root/'state'
            for p in (base,data,state): p.mkdir()
            old=base/'old'; new=base/'new'; old.mkdir();new.mkdir();(base/'current').symlink_to(old)
            db=data/'donetick.db'
            with sqlite3.connect(db) as c: c.execute('CREATE TABLE events(value TEXT)');c.execute("INSERT INTO events VALUES ('before')")
            calls=[]
            def control(action): calls.append(action)
            def migrate(release):
                self.assertEqual(release,new)
                with sqlite3.connect(db) as c: c.execute("INSERT INTO events VALUES ('migration')")
                if migration_fails: raise RuntimeError('migration')
            polled=False
            def healthy():
                nonlocal polled
                if not polled:
                    with sqlite3.connect(db) as c: c.execute("INSERT INTO events VALUES ('accepted update')")
                    polled=True
                return not health_fails
            with patch.multiple(m,BASE=base,DATA=data,STATE=state),patch.object(m.time,'sleep'),patch.object(m.os,'geteuid',return_value=1000),patch.object(m.shutil,'disk_usage',return_value=type('Disk',(),{'free':0 if disk_fails else 10*1024**3})()):
                if migration_fails or health_fails or disk_fails:
                    with self.assertRaises(RuntimeError): m.activate(new,migrate,healthy,control)
                else: m.activate(new,migrate,healthy,control)
            with sqlite3.connect(db) as c: events=[r[0] for r in c.execute('SELECT value FROM events')]
            if disk_fails:
                self.assertEqual(calls,[]);self.assertEqual(events,['before'])
            elif migration_fails:
                self.assertEqual(events,['before']);self.assertEqual((base/'current').resolve(),old)
                self.assertEqual(calls,['stop','start'])
            elif health_fails:
                self.assertIn('accepted update',events);self.assertEqual(calls,['stop','start','stop'])
            else:
                self.assertEqual((base/'current').resolve(),new);self.assertEqual(calls,['stop','start'])
            self.assertEqual(len(list((state/'backups').glob('*.db'))),0 if disk_fails else 1)
    def test_low_disk_leaves_running_service_untouched(self): self.run_case(disk_fails=True)
    def test_healthy_release(self): self.run_case()
    def test_failed_migration_restores_only_pre_start_snapshot(self): self.run_case(migration_fails=True)
    def test_failed_readiness_never_discards_accepted_updates(self): self.run_case(health_fails=True)
    def test_retention_never_deletes_unrelated_paths(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);base=root/'opt';state=root/'state'
            releases=base/'releases';backups=state/'backups';releases.mkdir(parents=True);backups.mkdir(parents=True)
            dirs=[]
            for n in range(5):
                p=releases/(str(n)*40);p.mkdir();m.os.utime(p,ns=(n+1,n+1));dirs.append(p)
            (releases/'unrelated').mkdir()
            for n in range(10): (backups/(str(n)+'.db')).write_bytes(b'fixture')
            (backups/'unrelated.db').write_bytes(b'keep')
            with patch.multiple(m,BASE=base,STATE=state): m.prune(dirs[0])
            self.assertTrue(dirs[0].exists());self.assertFalse(dirs[1].exists())
            self.assertTrue((releases/'unrelated').exists());self.assertTrue((backups/'unrelated.db').exists())
            self.assertEqual(len(list(backups.glob('*.db'))),8)
if __name__=='__main__': unittest.main()
