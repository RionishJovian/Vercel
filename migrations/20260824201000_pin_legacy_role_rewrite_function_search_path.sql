-- Security advisory remediation: this legacy role-rename helper remains in the
-- live database but is not part of the current application flow. Pin its
-- search path so object lookup cannot be influenced by the calling session.
-- The helper uses only built-in functions and schema-qualified application types.
ALTER FUNCTION private.rewrite_owner_app_role_expr(text)
  SET search_path TO '';
