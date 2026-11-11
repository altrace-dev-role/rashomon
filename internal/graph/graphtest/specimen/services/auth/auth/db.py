from auth.conn import connection


def insert_session(user, org, expires_at):
    conn = connection()
    return conn.execute("INSERT INTO public.sessions (user_id, org_id, expires_at) VALUES (%s, %s, %s)", user.id, org.id, expires_at)
