"""Private receipt images and review version checks."""
from alembic import op,context
import sqlalchemy as sa
revision='011'
down_revision='010'
branch_labels=None
depends_on=None
def upgrade():
 op.add_column('orders',sa.Column('receipt_revision',sa.Integer(),nullable=False,server_default='0'))
 op.create_table('receipt_images',sa.Column('id',sa.String(36),primary_key=True),sa.Column('order_id',sa.String(36),sa.ForeignKey('orders.id'),nullable=False),sa.Column('uploaded_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('request_key',sa.String(80),nullable=False),sa.Column('original_hash',sa.String(64),nullable=False),sa.Column('image',sa.LargeBinary(),nullable=False),sa.Column('width',sa.Integer(),nullable=False),sa.Column('height',sa.Integer(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.UniqueConstraint('order_id','request_key'),sa.CheckConstraint('width > 0 AND height > 0'))
 op.create_index('ix_receipt_images_order_id','receipt_images',['order_id'])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Receipt downgrade needs preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM receipt_images')).scalar():raise RuntimeError('Receipt records exist; preserve evidence and roll forward')
 op.drop_index('ix_receipt_images_order_id',table_name='receipt_images');op.drop_table('receipt_images')
 with op.batch_alter_table('orders') as batch:batch.drop_column('receipt_revision')
