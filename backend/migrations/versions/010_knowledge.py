"""Platform guidance and initial reviewed documentation cards."""
from alembic import op,context
import sqlalchemy as sa
revision='010'
down_revision='009'
branch_labels=None
depends_on=None
SEED=[{'id': 'guide-hotspot-radius', 'title': 'Portal, HotSpot and RADIUS: three jobs', 'category': 'Internet access', 'fact': 'The captive portal is the guest webpage. MikroTik HotSpot controls network access. RADIUS checks credentials, supplies access rules and receives usage reports.', 'why': 'Payment approval creates a stored access entitlement; the network adapter still needs to provision credentials and enforcement before a guest can use it.', 'requirements': 'HotSpot can use local accounts without RADIUS. Cloud RADIUS or local User Manager are authentication choices; their adapters are not connected in this build.', 'action': 'Captive portal', 'source_url': 'https://manual.mikrotik.com/docs/authentication-authorization-accounting/radius/', 'verified_on': '2026-10-05'}, {'id': 'guide-local-radius', 'title': 'Your router may host a RADIUS server', 'category': 'MikroTik', 'fact': 'MikroTik User Manager is a separate RADIUS server package. The documented active-session limits are Level 4: 20, Level 5: 50 and Level 6: unlimited by license.', 'why': 'At approximately 50 guests, a Level 4 router cannot host all those User Manager sessions. Multiple connections from one guest count as separate sessions.', 'requirements': 'Check exact model, RouterOS version, architecture, storage and license. User Manager excludes SMIPS. Unlimited licensing does not guarantee hardware capacity. Local integration is proposed, not implemented.', 'action': 'Routers', 'source_url': 'https://manual.mikrotik.com/docs/getting-started/routeros-licensing/mikrotik-hardware/', 'verified_on': '2026-10-05'}, {'id': 'guide-wireguard', 'title': 'WireGuard connects your router to Magic Hub', 'category': 'MikroTik', 'fact': 'WireGuard creates an encrypted tunnel and authenticates peers using keys. Guest voucher authentication is a separate HotSpot/RADIUS operation.', 'why': 'Our proposed configuration uses the tunnel for router management and cloud RADIUS traffic, while guest internet stays on the site WAN or Starlink.', 'requirements': 'Requires supported RouterOS, configured keys, routes and firewall isolation. A registered router or exported onboarding plan does not prove a live tunnel or connectivity.', 'action': 'Routers', 'source_url': 'https://manual.mikrotik.com/docs/virtual-private-networks/wireguard/', 'verified_on': '2026-10-05'}]
def upgrade():
 table=op.create_table('knowledge_cards',sa.Column('id',sa.String(36),primary_key=True),sa.Column('content',sa.JSON(),nullable=False),sa.Column('published',sa.Boolean(),nullable=False),sa.Column('archived',sa.Boolean(),nullable=False),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False),sa.CheckConstraint('revision > 0'))
 # JSON literals require explicit SQL representation for offline PostgreSQL generation.
 if context.is_offline_mode():
  import json
  for card in SEED:
   content={k:v for k,v in card.items() if k!='id'}
   op.execute(sa.text("INSERT INTO knowledge_cards (id, content, published, archived, revision, updated) VALUES ('"+card['id']+"', '"+json.dumps(content).replace("'","''")+"', true, false, 1, 1791158400)"))
 else:op.bulk_insert(table,[{'id':c['id'],'content':{k:v for k,v in c.items() if k!='id'},'published':True,'archived':False,'revision':1,'updated':1791158400} for c in SEED])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Guidance downgrade needs preservation checks')
 table=sa.table('knowledge_cards',sa.column('id'),sa.column('content',sa.JSON()),sa.column('revision'),sa.column('published'),sa.column('archived'))
 seeds={c['id']:{k:v for k,v in c.items() if k!='id'} for c in SEED}
 for row in op.get_bind().execute(sa.select(table)).mappings():
  if row['id'] not in seeds or row['content']!=seeds[row['id']] or row['revision']!=1 or not row['published'] or row['archived']:raise RuntimeError('Guidance records exist; preserve content and roll forward')
 op.drop_table('knowledge_cards')
