"""Create a credential/database-free development archive."""
from pathlib import Path
import zipfile
root=Path(__file__).resolve().parents[1]
excluded={'.venv','.runtime','node_modules','dist','__pycache__','.git','.ui-check','_qa'}
with zipfile.ZipFile(root.parent/'MikroMagic-development.zip','w',zipfile.ZIP_DEFLATED) as output:
 for path in root.rglob('*'):
  relative=path.relative_to(root)
  if not path.is_file() or excluded.intersection(relative.parts) or path.suffix in {'.sqlite3','.db','.pyc'} or path.name.endswith(('-wal','-shm')) or path.name.startswith('.env'):continue
  output.write(path,Path('mikromagic')/relative)
print('Development source archive generated; runtime data and environment files excluded.')
