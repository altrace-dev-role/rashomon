import redis

CLIENT = redis.Redis()


def put_session(session, ttl):
    CLIENT.set(session.id, session.token, ex=3600)
