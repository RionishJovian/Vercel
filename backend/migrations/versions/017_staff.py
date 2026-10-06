"""Business-owner delegated staff profiles."""
from alembic import op,context
import sqlalchemy as sa
revision='017'
down_revision='016'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('staff_grants',sa.Column('user_id',sa.String(36),sa.ForeignKey('users.id'),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('profile',sa.String(20),nullable=False),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('created_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('request_key',sa.String(80),nullable=False),sa.Column('request_hash',sa.String(64),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False),sa.UniqueConstraint('created_by','request_key'),sa.CheckConstraint("profile in ('manager','cashier','technician')"))
 op.create_index('ix_staff_grants_tenant_id','staff_grants',['tenant_id'])
 op.add_column('staff_grants',sa.Column('last_reason',sa.String(1000),nullable=False,server_default=''))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Staff downgrade requires preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM staff_grants')).scalar():raise RuntimeError('Staff grants exist; removing them would escalate accounts to owner authority')
 op.drop_index('ix_staff_grants_tenant_id',table_name='staff_grants');op.drop_table('staff_grants')
