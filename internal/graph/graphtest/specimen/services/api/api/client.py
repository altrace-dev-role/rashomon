import requests

AUTH_URL = "http://auth:8080"


def fetch_session(session_id):
    return requests.get(AUTH_URL + "/internal/sessions/" + session_id)
