"""Fixed private HQ evidence facade; actor/scope/capability are never body inputs."""
def schemas(obj,string,opaque):
    request=obj({k:opaque for k in ("case_owner","case_kind","case_reference","subject_type","subject_reference","media_object_id")})
    token=string(pattern="^[A-Za-z0-9_-]{43}$")
    content=obj({**request["properties"],"token":token})
    grant=obj({"contract_version":{"const":"media.admin-case-content.v1"},"id":string(format="uuid"),"token":token,"media_object_id":opaque,"expires_at":string(format="date-time")})
    return {"MediaCaseIssueRequest":request,"MediaCaseContentRequest":content,"MediaCaseGrant":grant,"MediaCaseRevokeRequest":obj({"reason_code":opaque}),"MediaCaseRevocation":obj({"status":{"const":"REVOKED"}})}

def request_schema(identifier):
    return {"media.case.grant.issue":"MediaCaseIssueRequest","media.case.content.read":"MediaCaseContentRequest","media.case.grant.revoke":"MediaCaseRevokeRequest"}[identifier]
