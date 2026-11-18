#!/usr/bin/env python3
"""Run with the pinned STT venv. Downloads only the selected multilingual model."""
import os
from huggingface_hub import snapshot_download
revision = '536b0662742c02347bc0e980a01041f333bce120'
root = os.environ['STT_MODEL_DIR']
# Files required by CTranslate2/faster-whisper; no arbitrary remote code.
snapshot_download('Systran/faster-whisper-small', revision=revision, cache_dir=root,
                  allow_patterns=['model.bin', 'config.json', 'tokenizer.json', 'vocabulary.*'])
print('Pinned multilingual small model ready')
