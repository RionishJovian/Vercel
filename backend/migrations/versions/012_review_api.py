"""Delegated service credentials and immutable external decision history."""
from alembic import op,context
import sqlalchemy as sa
revision='012'
down_revision='011'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('review_credentials',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('user_id',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('label',sa.String(120),nullable=False),sa.Column('provider',sa.String(16),nullable=False),sa.Column('reviewer_identity',sa.String(160),nullable=False),sa.Column('can_decide',sa.Boolean(),nullable=False),sa.Column('token_hash',sa.String(64),nullable=False,unique=True),sa.Column('created',sa.BigInteger(),nullable=False),sa.Column('expires',sa.BigInteger(),nullable=False),sa.Column('revoked_at',sa.BigInteger(),nullable=True),sa.Column('revoke_reason',sa.String(1000),nullable=True),sa.CheckConstraint('expires > created'))
 op.create_index('ix_review_credentials_tenant_id','review_credentials',['tenant_id'])
 op.create_table('external_review_actions',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('credential_id',sa.String(36),sa.ForeignKey('review_credentials.id'),nullable=False),sa.Column('order_id',sa.String(36),sa.ForeignKey('orders.id'),nullable=False),sa.Column('request_key',sa.String(80),nullable=False),sa.Column('request_hash',sa.String(64),nullable=False),sa.Column('reviewer_identity',sa.String(160),nullable=False),sa.Column('provider',sa.String(16),nullable=False),sa.Column('service_label',sa.String(120),nullable=False),sa.Column('decision',sa.String(24),nullable=False),sa.Column('reason',sa.String(1000),nullable=False),sa.Column('result',sa.JSON(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('credential_id','request_key'))
 op.create_index('ix_external_review_actions_tenant_id','external_review_actions',['tenant_id'])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Review API downgrade needs preservation checks')
 for table in ['external_review_actions','review_credentials']:
  if op.get_bind().execute(sa.text('SELECT count(*) FROM '+table)).scalar():raise RuntimeError('Review integration records exist; preserve credentials and audit history')
 for table in ['external_review_actions','review_credentials']:
  op.drop_index('ix_'+table+'_tenant_id',table_name=table);op.drop_table(table)
