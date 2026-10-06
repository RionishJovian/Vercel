"""Encrypted authenticator enrollment and single-use recovery records."""
from alembic import op,context
import sqlalchemy as sa
revision='019'
down_revision='018'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('account_mfa',sa.Column('user_id',sa.String(36),sa.ForeignKey('users.id'),primary_key=True),sa.Column('enabled',sa.Boolean(),nullable=False),sa.Column('encrypted_secret',sa.String(1000),nullable=True),sa.Column('enrollment_expires',sa.BigInteger(),nullable=True),sa.Column('last_step',sa.BigInteger(),nullable=False),sa.Column('recovery_hashes',sa.JSON(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('MFA downgrade requires preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM account_mfa')).scalar():raise RuntimeError('MFA records exist; preserve authentication and roll forward')
 op.drop_table('account_mfa')
