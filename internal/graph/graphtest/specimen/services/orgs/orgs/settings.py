from orgs.models import OrgSettings


def get_org_settings(org_id):
    return OrgSettings.for_org(org_id)
