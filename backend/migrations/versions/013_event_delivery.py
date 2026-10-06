"""Per-service event leases and repeatable payment lifecycle events."""
from alembic import op,context
import sqlalchemy as sa
revision='013'
down_revision='012'
branch_labels=None
depends_on=None
CONVENTION={'uq':'uq_%(table_name)s_%(column_0_name)s_%(column_1_name)s'}
def old_unique():
 if context.is_offline_mode():return 'outbox_events_order_id_event_key'
 for c in sa.inspect(op.get_bind()).get_unique_constraints('outbox_events'):
  if c['column_names']==['order_id','event']:return c['name'] or 'uq_outbox_events_order_id_event'
 raise RuntimeError('Expected outbox event constraint is missing')
def upgrade():
 op.add_column('orders',sa.Column('event_revision',sa.Integer(),nullable=False,server_default='0'))
 op.add_column('review_credentials',sa.Column('receive_events',sa.Boolean(),nullable=False,server_default=sa.false()))
 with op.batch_alter_table('outbox_events',naming_convention=CONVENTION) as batch:
  batch.drop_constraint(old_unique(),type_='unique')
  batch.add_column(sa.Column('revision',sa.Integer(),nullable=True));batch.add_column(sa.Column('created',sa.BigInteger(),nullable=True));batch.add_column(sa.Column('payload',sa.JSON(),nullable=True))
  batch.create_unique_constraint('uq_outbox_order_revision',['order_id','revision'])
 op.create_table('event_deliveries',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('credential_id',sa.String(36),sa.ForeignKey('review_credentials.id'),nullable=False),sa.Column('event_id',sa.String(36),sa.ForeignKey('outbox_events.id'),nullable=False),sa.Column('state',sa.String(16),nullable=False),sa.Column('attempts',sa.Integer(),nullable=False),sa.Column('cycle_attempts',sa.Integer(),nullable=False),sa.Column('available_at',sa.BigInteger(),nullable=False),sa.Column('lease_expires',sa.BigInteger(),nullable=True),sa.Column('lease_hash',sa.String(64),nullable=True),sa.Column('completion',sa.JSON(),nullable=True),sa.Column('acknowledged_at',sa.BigInteger(),nullable=True),sa.Column('last_error',sa.String(1000),nullable=True),sa.UniqueConstraint('credential_id','event_id'),sa.CheckConstraint("state IN ('pending','leased','acknowledged','failed')"),sa.CheckConstraint('attempts >= 0 AND cycle_attempts >= 0'))
 op.create_index('ix_event_deliveries_tenant_id','event_deliveries',['tenant_id']);op.create_index('ix_event_deliveries_credential_id','event_deliveries',['credential_id'])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Delivery downgrade needs preservation checks')
 bind=op.get_bind()
 if bind.execute(sa.text('SELECT count(*) FROM event_deliveries')).scalar() or bind.execute(sa.text('SELECT count(*) FROM outbox_events WHERE revision IS NOT NULL')).scalar():raise RuntimeError('Delivery/event records exist; preserve lifecycle history and roll forward')
 op.drop_index('ix_event_deliveries_credential_id',table_name='event_deliveries');op.drop_index('ix_event_deliveries_tenant_id',table_name='event_deliveries');op.drop_table('event_deliveries')
 with op.batch_alter_table('outbox_events') as batch:
  batch.drop_constraint('uq_outbox_order_revision',type_='unique');batch.drop_column('revision');batch.drop_column('created');batch.drop_column('payload');batch.create_unique_constraint('uq_outbox_events_order_id_event',['order_id','event'])
 with op.batch_alter_table('review_credentials',table_args=[sa.CheckConstraint('expires > created')]) as batch:batch.drop_column('receive_events')
 with op.batch_alter_table('orders',table_args=[sa.CheckConstraint('amount > 0')]) as batch:batch.drop_column('event_revision')
