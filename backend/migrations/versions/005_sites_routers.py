"""Sites, unverified router registry and branch portal configuration."""
from alembic import op,context
import sqlalchemy as sa
revision='005'
down_revision='004'
branch_labels=None
depends_on=None

def upgrade():
 op.create_table('sites',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('name',sa.String(120),nullable=False),sa.Column('timezone',sa.String(80),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('tenant_id','name'))
 op.create_index('ix_sites_tenant_id','sites',['tenant_id'])
 op.create_table('router_devices',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('site_id',sa.String(36),sa.ForeignKey('sites.id'),nullable=False),sa.Column('name',sa.String(120),nullable=False),sa.Column('model',sa.String(120),nullable=False),sa.Column('reported_routeros',sa.String(40),nullable=False),sa.Column('registered_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('site_id','name'))
 op.create_index('ix_router_devices_tenant_id','router_devices',['tenant_id'])
 op.create_table('site_portals',sa.Column('site_id',sa.String(36),sa.ForeignKey('sites.id'),primary_key=True),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('draft',sa.JSON(),nullable=False),sa.Column('published',sa.JSON(),nullable=True),sa.Column('published_revision',sa.Integer(),nullable=True),sa.Column('updated',sa.BigInteger(),nullable=False))
 for table in ['orders','vouchers','entitlements']:
  with op.batch_alter_table(table,naming_convention={'ck':'ck_%(table_name)s'}) as batch:
   batch.add_column(sa.Column('site_id',sa.String(36),nullable=True))
   batch.create_foreign_key('fk_'+table+'_site','sites',['site_id'],['id'])

def downgrade():
 if context.is_offline_mode():raise RuntimeError('Site downgrade needs online preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM sites')).scalar():raise RuntimeError('Site records exist; preserve branches and roll forward')
 for table in ['orders','vouchers','entitlements']:
  with op.batch_alter_table(table,naming_convention={'ck':'ck_%(table_name)s'}) as batch:
   batch.drop_constraint('fk_'+table+'_site',type_='foreignkey');batch.drop_column('site_id')
 op.drop_table('site_portals');op.drop_index('ix_router_devices_tenant_id',table_name='router_devices');op.drop_table('router_devices');op.drop_index('ix_sites_tenant_id',table_name='sites');op.drop_table('sites')
