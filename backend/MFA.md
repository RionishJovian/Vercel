# Optional authenticator setup

Set `MIKROMAGIC_MFA_KEY` to a separately generated Fernet key before starting the account API. Never put this key in the source archive, browser configuration, logs or database. Keep a protected backup separately from database backups. Restart processes after configuring it. Existing secrets cannot be decrypted with a replacement key; key rotation and operator break-glass recovery are not implemented.

Accounts enroll with their current password, manually enter the private setup secret in an authenticator, then confirm a six-digit code within ten minutes. Confirmation ends existing account sessions and returns a replacement session plus ten recovery codes, displayed once. Store those codes privately. Recovery codes are hashed and consumed once. Password resets preserve MFA. Disabling requires the password and an unused authenticator/recovery code, ends sessions and clears stored MFA secrets. Existing delegated review service credentials remain independently managed.

Authenticator codes follow RFC 6238 (SHA-1, six digits, 30-second steps); the server accepts the preceding/current/following step and refuses a consumed or older step. Maintain synchronized server and device clocks. TLS, protected environment secrets, production database concurrency checks and authenticated/mobile browser QA are required before production use. This optional feature does not enforce MFA for all accounts. Guest sign-in also accepts an authenticator or recovery code.

Sources: https://www.rfc-editor.org/rfc/rfc6238.html and https://cryptography.io/en/stable/fernet/ .
