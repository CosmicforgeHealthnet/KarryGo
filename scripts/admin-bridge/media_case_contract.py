"""Pin the private, default-disabled Media case transport; no grants are approved."""
import argparse
import json
from pathlib import Path
from contracts import obj, string
from openapi_spec_validator import validate_spec
ROOT=Path(__file__).resolve().parents[2]
TARGET=ROOT/"services/media-file-service/docs/openapi/admin-case-access.openapi.json"

def document():
 opaque=string(pattern=r"^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$",maxLength=128)
 values={"type":["array","null"],"maxItems":128,"uniqueItems":True,"items":opaque}
 actor=obj({"product":{"const":"karryam"},"staff":opaque,"expires_at":string(format="date-time"),**{k:values for k in ("roles","permissions","market_scopes","zone_scopes","organization_scopes","tenant_scopes")}},["product","staff","expires_at"])
 scope=obj({"product":{"const":"karryam"},**{k:opaque for k in ("market","zone","organization","tenant")}})
 request=obj({"scope":scope,**{k:opaque for k in ("purpose","source_capability","case_owner","case_kind","case_reference","subject_type","subject_reference","media_object_id")}})
 token=string(pattern=r"^[A-Za-z0-9_-]{43}$")
 grant=obj({"contract_version":{"const":"media.admin-case-content.v1"},"id":string(format="uuid"),"token":token,"media_object_id":opaque,"expires_at":string(format="date-time")})
 def op(name,body,response,parameters=[]):
  return {"operationId":name,"description":"Default disabled. Dedicated Bridge workload credential and durable namespaced nonce; metadata comes from existing authenticated staff assertions. No browser/native owner credential reuse. Native signed policy and a live authoritative source-case adapter are mandatory for issuance/redemption. Production source adapters remain unenrolled; flags alone cannot grant access.","parameters":parameters,"requestBody":{"required":True,"content":{"application/json":{"schema":body}}},"responses":{"200":{"description":"Authorized private result; no storage URL or raw source evidence metadata","content":response},**{str(c):{"description":"Sanitized denial/unavailable, no content or grant"} for c in (400,401,403,404,409,413,503)}},"x-strict-json":True,"x-max-request-bytes":32768}
 base="/internal/media/v1/admin/case-grants"
 parameter=[{"name":"id","in":"path","required":True,"schema":string(format="uuid")}]
 issue=op("media.case.grants.issue",obj({"actor":actor,"request":request}),{"application/json":{"schema":grant}})
 read=op("media.case.grants.redeem",obj({"actor":actor,"request":request,"token":token}),{"application/octet-stream":{"schema":{"type":"string","format":"binary"}}},parameter)
 read["responses"]["200"]["headers"]={"X-Media-Case-Contract":{"schema":{"const":"media.admin-case-content.v1"}},"X-Media-Object-ID":{"schema":opaque}}
 revoke=op("media.case.grants.withdraw",obj({"actor":actor,"reason_code":opaque}),{"application/json":{"schema":obj({"status":{"const":"REVOKED"}})}},parameter)
 revoke["description"]+=" Authenticated staff may withdraw only their own grant after read kills or case closure. This can only reduce access."
 return {"openapi":"3.1.0","info":{"title":"Media private case-bound content transport","version":"1.0.0"},"security":[{"CaseWorkloadHMAC":[]}],"paths":{base:{"post":issue},base+"/{id}/content":{"post":read},base+"/{id}/revocations":{"post":revoke}},"components":{"securitySchemes":{"CaseWorkloadHMAC":{"type":"apiKey","in":"header","name":"X-Service-Signature","description":"HMAC v2 exact method/path/query/body plus service-name/version/timestamp/nonce/signature headers. Fixed caller admin-backoffice-service, distinct case key, timestamp window 30 seconds, durable replay protection. Current staff metadata is trusted only from that authenticated workload; the Bridge must validate its HQ staff assertion on every request."}}}}

def main():
 parser=argparse.ArgumentParser();parser.add_argument("--write",action="store_true");args=parser.parse_args()
 data=document();validate_spec(data);raw=(json.dumps(data,indent=2)+"\n").encode()
 if args.write:TARGET.parent.mkdir(parents=True,exist_ok=True);TARGET.write_bytes(raw)
 else:assert TARGET.read_bytes()==raw,"Media case source contract drift"
 print("Private Media case OpenAPI validated; live source adapters and HQ enrollment remain disabled")
if __name__=="__main__":main()
