"""Source-owned review queue lifecycle, distinct from account decisions."""
import copy

def build(obj,string,opaque,operation):
    identifier=string(format="uuid")
    command=obj({"contract_version":{"const":"customer.admin-review-work-item.v1"},"operation_id":identifier,"review_request_id":identifier,"customer_id":identifier,"expected_version":{"type":"integer","minimum":1},"action":{"enum":["ACKNOWLEDGE","CLOSE"]},"idempotency_key":opaque,"reason_code":opaque,"requested_at":string(format="date-time")})
    receipt=obj({"contract_version":{"const":"customer.admin-review-work-item.v1"},"operation_id":identifier,"review_request_id":identifier,"customer_id":identifier,"idempotency_key":opaque,"intent_digest":string(pattern="^[0-9a-f]{64}$"),"state":{"enum":["ACKNOWLEDGED","CLOSED"]},"recorded_version":{"type":"integer","minimum":2},"outcome":{"enum":["REVIEW_ACKNOWLEDGED","REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE"]},"scope":obj({"product":{"const":"karryam"},**{k:opaque for k in ("market","zone","organization","tenant")}}),"recorded_at":string(format="date-time")})
    receipt["allOf"]=[{"if":{"properties":{"state":{"const":"ACKNOWLEDGED"}}},"then":{"properties":{"outcome":{"const":"REVIEW_ACKNOWLEDGED"}}},"else":{"properties":{"outcome":{"const":"REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE"}}}}]
    result=copy.deepcopy(operation);result["properties"]["receipt"]=receipt
    return {"CustomerWorkItemCommand":command,"CustomerWorkItemReceipt":receipt,"CustomerWorkItemOperation":result}
