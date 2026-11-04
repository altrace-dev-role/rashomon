from auth.conn import connection
from auth.sessions import SESSION_TTL_SECONDS


def purge():
    conn = connection()
    conn.execute("DELETE FROM public.sessions WHERE created_at < now() - interval '%s seconds'", SESSION_TTL_SECONDS)
