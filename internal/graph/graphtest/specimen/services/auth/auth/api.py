from auth.sessions import create_session


def create_session_handler(request):
    user = request.user
    org = request.org
    return create_session(user, org)
