"""Platform appearance draft, publication and one-step restoration."""
from alembic import op,context
import sqlalchemy as sa
revision='008'
down_revision='007'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('theme_settings',sa.Column('id',sa.String(32),primary_key=True),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('draft',sa.JSON(),nullable=False),sa.Column('published',sa.JSON(),nullable=True),sa.Column('previous',sa.JSON(),nullable=True),sa.Column('published_revision',sa.Integer(),nullable=True),sa.Column('updated',sa.BigInteger(),nullable=False))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Appearance downgrade needs preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM theme_settings')).scalar():raise RuntimeError('Appearance records exist; preserve publication and roll forward')
 op.drop_table('theme_settings')
