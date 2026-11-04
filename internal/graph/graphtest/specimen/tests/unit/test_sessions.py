from auth.sessions import create_session


def test_create(user, org):
    session = create_session(user, org)
    assert session.expires_at
