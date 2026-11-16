#!/usr/bin/env python3
"""Local faster-whisper adapter. Audio is a temporary file, deleted by caller."""
import json, os, sys
os.environ.setdefault('HF_HUB_DISABLE_TELEMETRY', '1')
try:
    from faster_whisper import WhisperModel
    model = WhisperModel(os.environ.get('STT_MODEL', 'small'), device='cpu',
                         compute_type='int8', cpu_threads=2,
                         download_root=os.environ.get('STT_MODEL_DIR'), local_files_only=True)
    segments, info = model.transcribe(sys.argv[1], language='ru', beam_size=5, vad_filter=True)
    if info.duration > 60:
        raise ValueError('duration_limit')
    text = ' '.join(segment.text.strip() for segment in segments)
    print(json.dumps({'text': text}, ensure_ascii=False))
except Exception:
    # Do not print exceptions, paths or audio content to diagnostic stderr.
    print('STT unavailable or audio exceeds limits', file=sys.stderr)
    sys.exit(1)
