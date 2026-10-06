"""Trusted offline account provisioning; no public self-escalation endpoint."""
import argparse,os,getpass
from sqlalchemy import create_engine,select
from sqlalchemy.orm import Session
from .models import Tenant,User
from .app import HASHER
p=argparse.ArgumentParser();p.add_argument('--username',required=True);p.add_argument('--role',choices=['platform','sme','guest'],required=True);p.add_argument('--tenant-name');a=p.parse_args()
url=os.environ.get('DATABASE_URL');assert url,'Set DATABASE_URL';assert a.role=='platform' or a.tenant_name,'SME/guest need --tenant-name'
password=getpass.getpass('New account password: ');assert len(password)>=12,'Use at least 12 characters';assert password==getpass.getpass('Repeat password: '),'Passwords differ'
with Session(create_engine(url)) as s:
 t=None
 if a.tenant_name:
  t=s.scalar(select(Tenant).where(Tenant.name==a.tenant_name))
  if not t:t=Tenant(name=a.tenant_name);s.add(t);s.flush()
 s.add(User(username=a.username.lower().strip(),role=a.role,tenant_id=t.id if t else None,password_hash=HASHER.hash(password)));s.commit()
print('Account created. Password was not printed or stored in plaintext.')
