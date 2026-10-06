"""Platform-owned hubs, business assignments and immutable router plans."""
from alembic import op,context
import sqlalchemy as sa
revision='006'
down_revision='005'
branch_labels=None
depends_on=None

def upgrade():
 op.create_table('magic_hubs',sa.Column('id',sa.String(36),primary_key=True),sa.Column('name',sa.String(120),nullable=False,unique=True),sa.Column('endpoint',sa.String(253),nullable=False),sa.Column('port',sa.Integer(),nullable=False),sa.Column('pool',sa.String(18),nullable=False,unique=True),sa.Column('public_key',sa.String(44),nullable=False,unique=True),sa.Column('next_host',sa.Integer(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('endpoint','port'),sa.CheckConstraint('port >= 1 AND port <= 65535'),sa.CheckConstraint('next_host >= 2 AND next_host <= 255'))
 op.create_table('hub_assignments',sa.Column('hub_id',sa.String(36),sa.ForeignKey('magic_hubs.id'),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),primary_key=True),sa.Column('created',sa.BigInteger(),nullable=False))
 op.create_table('router_plans',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('router_id',sa.String(36),sa.ForeignKey('router_devices.id'),nullable=False,unique=True),sa.Column('hub_id',sa.String(36),sa.ForeignKey('magic_hubs.id'),nullable=False),sa.Column('router_public_key',sa.String(44),nullable=False,unique=True),sa.Column('address',sa.String(15),nullable=False),sa.Column('snapshot',sa.JSON(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.Column('reviewed_at',sa.BigInteger(),nullable=True),sa.UniqueConstraint('hub_id','address'))
 op.create_index('ix_router_plans_tenant_id','router_plans',['tenant_id'])

def downgrade():
 if context.is_offline_mode():raise RuntimeError('Hub downgrade needs online preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM magic_hubs')).scalar():raise RuntimeError('Hub records exist; preserve plans and roll forward')
 op.drop_index('ix_router_plans_tenant_id',table_name='router_plans');op.drop_table('router_plans');op.drop_table('hub_assignments');op.drop_table('magic_hubs')
