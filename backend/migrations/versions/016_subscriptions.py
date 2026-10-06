"""Platform-authored subscription plans and tenant records."""
from alembic import op,context
import sqlalchemy as sa
revision='016'
down_revision='015'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('subscription_plans',sa.Column('id',sa.String(36),primary_key=True),sa.Column('settings',sa.JSON(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False))
 op.create_table('tenant_subscriptions',sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),primary_key=True),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('plan_id',sa.String(36),sa.ForeignKey('subscription_plans.id'),nullable=False),sa.Column('plan_snapshot',sa.JSON(),nullable=False),sa.Column('state',sa.String(20),nullable=False),sa.Column('period_start',sa.BigInteger(),nullable=False),sa.Column('period_end',sa.BigInteger(),nullable=False),sa.Column('reason',sa.String(1000),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False),sa.CheckConstraint("state in ('trial','active','past_due','suspended','cancelled')"),sa.CheckConstraint('period_end > period_start'))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Subscription downgrade requires preservation checks')
 bind=op.get_bind()
 if bind.execute(sa.text('SELECT count(*) FROM tenant_subscriptions')).scalar() or bind.execute(sa.text('SELECT count(*) FROM subscription_plans')).scalar():raise RuntimeError('Subscription records exist; preserve them and roll forward')
 op.drop_table('tenant_subscriptions');op.drop_table('subscription_plans')
