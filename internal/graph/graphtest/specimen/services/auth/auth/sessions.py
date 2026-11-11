from datetime import datetime, timedelta

from auth.cache import put_session
from auth.db import insert_session

SESSION_TTL_SECONDS = 3600


def create_session(user, org):
    expires_at = datetime.utcnow() + timedelta(seconds=SESSION_TTL_SECONDS)
    session = insert_session(user, org, expires_at)
    put_session(session, SESSION_TTL_SECONDS)
    return session


def is_expired(session):
    return session.expires_at < datetime.utcnow()
