"""Platform onboarding and first-login password changes."""
from alembic import op,context
import sqlalchemy as sa
revision='014'
down_revision='013'
branch_labels=None
depends_on=None
def upgrade():
 op.add_column('users',sa.Column('must_change_password',sa.Boolean(),nullable=False,server_default=sa.false()))
 op.add_column('users',sa.Column('temporary_password_expires',sa.BigInteger(),nullable=True))
 op.create_table('business_provisioning',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False,unique=True),sa.Column('owner_id',sa.String(36),sa.ForeignKey('users.id'),nullable=False,unique=True),sa.Column('created_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('request_key',sa.String(80),nullable=False),sa.Column('request_hash',sa.String(64),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('created_by','request_key'))
 op.create_table('password_reset_records',sa.Column('id',sa.String(36),primary_key=True),sa.Column('user_id',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('actor_id',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('reason',sa.String(1000),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Onboarding downgrade needs preservation checks')
 bind=op.get_bind()
 if bind.execute(sa.text('SELECT count(*) FROM business_provisioning')).scalar() or bind.execute(sa.text('SELECT count(*) FROM password_reset_records')).scalar() or bind.execute(sa.text('SELECT count(*) FROM users WHERE must_change_password=true')).scalar():raise RuntimeError('Onboarding/password records exist; preserve accounts and roll forward')
 op.drop_table('password_reset_records');op.drop_table('business_provisioning')
 with op.batch_alter_table('users',table_args=[sa.CheckConstraint("role in ('platform','sme','guest')"),sa.CheckConstraint("role = 'platform' OR tenant_id IS NOT NULL")]) as batch:batch.drop_column('temporary_password_expires');batch.drop_column('must_change_password')
