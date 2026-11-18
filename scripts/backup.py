#!/usr/bin/env python3
"""Consistent SQLite backup or restore. Stop backend before restoring."""
import os,sqlite3,sys
if len(sys.argv)!=3: sys.exit('Usage: backup.py SOURCE_DB NEW_DESTINATION_DB')
src,dst=sys.argv[1:]
if not os.path.isfile(src) or os.path.exists(dst): sys.exit('Source missing or destination exists')
os.umask(0o077)
with sqlite3.connect('file:'+os.path.abspath(src)+'?mode=ro',uri=True) as a,sqlite3.connect(dst) as b: a.backup(b)
