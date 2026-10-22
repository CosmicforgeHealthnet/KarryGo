"""Finite scoped component schemas; raw membership is never disclosed."""
REPORTS = {'mission-outcomes': ['committed', 'assigned', 'completed', 'failed', 'cancelled', 'recovery_resolved', 'recovery_failed', 'guarantee_breaches'], 'recovery': ['started', 'replaced', 'resolved', 'failed'], 'capability-mismatches': ['mismatches'], 'declines': ['declines', 'safety_declines'], 'demand': ['requests', 'unmatched'], 'demand-forecast': ['forecast_requests'], 'supply': ['available_providers', 'busy_providers'], 'provider-quality': ['completed', 'failed', 'cancelled', 'arrivals', 'declines', 'safety_declines', 'recoveries_as_original', 'recoveries_as_replacement'], 'recovery-resolution': ['resolved_with_duration', 'resolution_seconds_mean', 'resolution_seconds_median'], 'provider-quality-score': ['quality_scored_providers', 'quality_score_v1_mean_bp', 'quality_score_v1_median_bp'], 'readiness': ['ready_count', 'on_time_count', 'late_count', 'lateness_seconds_total', 'prep_seconds_total', 'prep_count']}
def schema_name(name):return "Scoped"+"".join(part.title() for part in name.split("-"))+"Release"
def permission(name):return "analytics.scoped."+("readiness-components" if name=="readiness" else name)+".read"
def schemas(base):
 import copy
 result={}
 for name,keys in REPORTS.items():
  schema=copy.deepcopy(base);p=schema["properties"];p["metric"]={"const":name};cells=p["cells"];cells["minItems"]=cells["maxItems"]=len(keys);cells["items"]["properties"]["key"]={"enum":keys};cells["allOf"]=[{"contains":{"properties":{"key":{"const":key}},"required":["key"]},"minContains":1,"maxContains":1} for key in keys];result[schema_name(name)]=schema
 return result
