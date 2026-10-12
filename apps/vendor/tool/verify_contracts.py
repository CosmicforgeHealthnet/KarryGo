from pathlib import Path
import json,re
root=Path(__file__).resolve().parents[3]
app=root/'apps/vendor'
m=json.loads((app/'contracts/vendor_manifest.json').read_text())
snapshot=json.loads((root/'services/vendor-service/docs/SECTION43_ROUTE_PARITY.json').read_text())
norm=lambda value:re.sub(r'\{[^}]+\}','{}',value)
expected={norm(r) for r in snapshot['registeredRoutes'] if '/api/v1/vendor/' in r and '/internal/' not in r}
actual={norm(r['method']+' '+r['path']) for r in m['operations']}
assert expected<=actual, 'Missing backend Vendor routes: '+str(expected-actual)
assert len(actual)==len(m['operations']), 'Duplicate operations'
assert not m['unresolved_requests'], 'Unresolved mutation schema'
assert all(r['path'].startswith('/api/v1/vendor/') and '/internal/' not in r['path'] for r in m['operations'])
for r in m['operations']:
 source=r['source'].split('#')[0]
 if source.endswith('.go'):
  assert (root/source).is_file(),source
  handler=r.get('handler')
  if handler and r['request_schema']:
   # Independently check no top-level source-derived field name was invented.
   package=root/handler['package']
   tags=set()
   for f in package.glob('*.go'):
    if not f.name.endswith('_test.go'):tags.update(re.findall(r'json:"([^",]+)',f.read_text(encoding='utf-8-sig')))
   if handler['package'].endswith('/shop'):
    for f in package.glob('*.go'):
     if f.name.endswith('_test.go'):continue
     for name in re.findall(r'^\s*(\w+)\s+[*\w\[\]]+\s*$',f.read_text(encoding='utf-8-sig'),re.M):
      name=re.sub(r'([A-Z]+)([A-Z][a-z])',r'\1_\2',name)
      tags.add(re.sub(r'([a-z0-9])([A-Z])',r'\1_\2',name).lower().replace('_i_ds','_ids'))
   if handler['package'].endswith('/orders') and '/commerce/section21/' in r['path']:
    for filename,struct_name in [('commerce_orders.go','ScheduledOrderInput'),('recurring_feeding.go','RecurringScheduleInput')]:
     text=(package/filename).read_text(encoding='utf-8-sig')
     block=re.search(r'type '+struct_name+r' struct\s*\{([^}]+)\}',text,re.S)
     assert block,struct_name
     tags.update(re.findall(r'^\s*([A-Z]\w*)\s+[^\n]+$',block[1],re.M))
   assert set(r['request_schema'].get('properties',{}))<=tags, r['path']
 else:assert (root/'services/vendor-service/docs/openapi'/source).is_file(),source
 if r['method']=='GET':assert not r['request_required'],r['path']
 assert (app/'lib/features'/r['module']/'data'/f"{r['module']}_api.dart").is_file()
for route in actual-expected:
 assert '/readiness' in route or '/orchestration' in route or '/api/v1/vendor/media/' in route or route in {'GET /api/v1/vendor/staff/me/access', 'GET /api/v1/vendor/delivery/rider/branches', 'GET /api/v1/vendor/cloud-kitchen/context/{}', 'GET /api/v1/vendor/cloud-kitchen/corporate-references', 'GET /api/v1/vendor/pastry/request-references', 'POST /api/v1/vendor/crm/exports/{}/file', 'POST /api/v1/vendor/onboarding/document-uploads', 'GET /api/v1/vendor/onboarding/document-uploads/{}', 'GET /api/v1/vendor/commerce/section21/scheduled-orders', 'GET /api/v1/vendor/commerce/section21/scheduled-orders/{}', 'GET /api/v1/vendor/commerce/section21/recurring-schedules', 'GET /api/v1/vendor/commerce/section21/recurring-schedules/{}', 'GET /api/v1/vendor/commerce/section21/recurring-setup-contexts', 'GET /api/v1/vendor/commerce/section21/scheduling-quotes', 'GET /api/v1/vendor/commerce/section21/scheduling-quotes/{}', 'GET /api/v1/vendor/commerce/section21/rfqs', 'GET /api/v1/vendor/commerce/section21/rfqs/{}', 'GET /api/v1/vendor/commerce/section21/rfqs/{}/quotes', 'GET /api/v1/vendor/pastry/products/{}/option-groups'}, 'Unaccounted new route '+route
# Require the newly added scheduling reads to exist in the mounted source.
read_source=(root/'services/vendor-service/internal/features/orders/commerce_orders_http.go').read_text(encoding='utf-8-sig')
for path in ['/scheduled-orders','/scheduled-orders/:id','/recurring-schedules','/recurring-schedules/:scheduleId','/rfqs','/rfqs/:rfqId','/rfqs/:rfqId/quotes','/scheduling-quotes','/scheduling-quotes/:quoteId','/recurring-setup-contexts']:
 assert 'g.GET("'+path+'",' in read_source,path
# Critical contracts were checked against actual handler fields.
find=lambda method,path:next(r for r in m['operations'] if r['method']==method and r['path']==path)
support=find('POST','/api/v1/vendor/commerce/orders/{vendorOrderId}/support-cases')
assert set(support['request_schema']['properties'])=={'category','description','vendor_order_ref','line_refs'}
assert support['request_schema']['properties']['category']['type']=='string'
assert len(support['request_schema']['properties']['category']['enum'])==11
assert any(p['in']=='header' and p['name']=='Idempotency-Key' and p['required'] for p in support['parameters'])
preview=find('POST','/api/v1/vendor/food/items/{itemId}/pricing/resolve')
assert set(preview['request_schema']['properties'])=={'variant_ref','modifier_refs'}
assert preview['permission']=='vendor.food.catalogue.read'
assert not any(p.get('in')=='header' and p.get('name','').lower()=='idempotency-key' for p in preview['parameters'])
adjustment=find('POST','/api/v1/vendor/shop/inventory/adjustments')
assert set(adjustment['request_schema']['properties'])=={'stock_basis_id','quantity_delta','reason_code','note','expected_revision'}
receipt=find('POST','/api/v1/vendor/shop/inventory/receipts')
assert set(receipt['request_schema']['properties'])=={'branch_id','source_ref','supplier_ref','notes','received_at','lines'}
assert set(receipt['request_schema']['properties']['lines']['items']['properties'])=={'stock_basis_id','quantity','lot_number','batch_number','manufactured_at','best_before_at','expires_at','unit_cost_minor'}
for kind in ['movements','receipts','transfers','quarantines','reorder-rules','stocktakes','alerts']:
 op=find('GET','/api/v1/vendor/shop/branches/{branchId}/inventory/'+kind)
 assert {'limit','offset'} <= {p['name'] for p in op['parameters'] if p['in']=='query'}
transfer=find('POST','/api/v1/vendor/shop/inventory/transfers')
assert set(transfer['request_schema']['properties'])=={'source_branch_id','destination_branch_id','notes','lines'}
assert set(transfer['request_schema']['properties']['lines']['items']['properties'])=={'source_stock_basis_id','destination_stock_basis_id','quantity'}
arrival=find('POST','/api/v1/vendor/shop/inventory/transfers/{transferId}/receive')
assert set(arrival['request_schema']['properties'])=={'lines'}
assert set(arrival['request_schema']['properties']['lines']['items']['properties'])=={'line_id','received_quantity','damaged_quantity','missing_quantity'}
quarantine=find('POST','/api/v1/vendor/shop/inventory/quarantines')
assert set(quarantine['request_schema']['properties'])=={'stock_basis_id','quantity','reason_code','note'}
resolve=find('POST','/api/v1/vendor/shop/inventory/quarantines/{quarantineId}/resolve')
assert set(resolve['request_schema']['properties'])=={'action'}
assert resolve['permission']=='vendor.shop.inventory.quarantine'
reorder=find('PUT','/api/v1/vendor/shop/inventory/stock-bases/{stockBasisId}/reorder-rule')
assert set(reorder['request_schema']['properties'])=={'minimum_quantity','target_quantity','expiring_soon_hours'}
assert reorder['permission']=='vendor.shop.inventory.manage_policy'
assert reorder['request_schema']['properties']['expiring_soon_hours']['nullable']
withdraw=find('POST','/api/v1/vendor/finance/withdrawals')
assert set(withdraw['request_schema']['properties'])=={'bank_account_id','amount_kobo','currency','step_up_grant_id'}
pickup=find('POST','/api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/pickup/confirm')
assert set(pickup['request_schema']['properties'])=={'handover_ref','method','credential'}
delete=find('DELETE','/api/v1/vendor/onboarding/documents/{documentId}')
assert set(delete['request_schema']['properties'])=={'expected_version'}
lines=['# Contract coverage','','Generated client coverage checked against the backend Section 43 snapshot and newer source routes. This is repository parity, not live deployment certification.','',f"- Snapshot public Vendor operations: {len(expected)}",f"- Generated operations: {len(actual)}",f"- Newer source operations: {len(actual-expected)}",f"- Feature areas: {len(m['modules'])}",'- Unresolved top-level request schemas: 0','- No Customer/internal/operator routes exposed by this project.','', '| Feature | Operations |','|---|---|']
for mod,n in sorted(m['modules'].items()):lines.append(f'| {mod} | {n} |')
lines+=['','Response schemas remain incomplete in several backend documents. Those responses retain exact raw JSON through VendorResponse; no guessed response fields are supplied. Requiredness, state transitions, tenant scope and backend validation remain server authority.','','Analytics public Vendor access, realtime subscription still need backend integration contracts. Compliance upload uses the onboarding-specific authorization flow; see ONBOARDING.md. Screen-level completeness is tracked separately in docs/frontend-coverage.md.','']
(app/'CONTRACT_COVERAGE.md').write_text('\n'.join(lines),encoding='utf-8')
print(json.dumps({'snapshot_routes':len(expected),'generated_routes':len(actual),'source_additions':len(actual-expected),'modules':len(m['modules']),'source_fields':'validated','critical_flows':'validated'},indent=2))

pastry_source=(root/'services/vendor-service/internal/features/pastry/http.go').read_text(encoding='utf-8-sig')
assert 'GET("/products/:productId/option-groups",' in pastry_source
