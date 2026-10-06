"""Regenerate the API contract from application routes without accessing live data."""
import json
from pathlib import Path
import sys
root=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(root))
from backend.app import make_app
(root/'backend/openapi.json').write_text(json.dumps(make_app('sqlite://').openapi(),indent=2)+'\n')
print('OpenAPI contract regenerated.')
