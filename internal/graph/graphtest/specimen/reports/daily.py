from auth.conn import connection


def active_sessions(org_id):
    conn = connection()
    return conn.execute("SELECT count(*) FROM public.sessions WHERE org_id = %s AND expires_at > now()", org_id)
