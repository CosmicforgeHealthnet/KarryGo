from pathlib import Path
import json,re,hashlib,sys
selected_modules=set(sys.argv[1:])
ROOT=Path(__file__).resolve().parents[3]
OUT=ROOT/'apps/vendor'
DOC=ROOT/'services/vendor-service/docs/openapi'
source_file=OUT/'tool/backend_ast_python.json'
source=json.loads(source_file.read_text(encoding='utf-8-sig'))
structs={(s['package'],s['name']):s for s in source['structs']}
aliases={(a['package'],a['name']):a['type'] for a in source.get('aliases',[])}
functions={}
for f in source['functions']:functions.setdefault((f['package'],f['name']),[]).append(f)

def resolve_struct(pkg,name):
 name=name.lstrip('*')
 if (pkg,name) in structs:return structs[pkg,name]
 last=name.split('.')[-1]
 candidates=[s for (p,n),s in structs.items() if n==last and (p==pkg or p.startswith(pkg+'/') or pkg.startswith(p+'/'))]
 if len(candidates)==1:return candidates[0]
 candidates=[s for (p,n),s in structs.items() if n==last]
 return candidates[0] if len(candidates)==1 else None

def go_schema(pkg,t,seen=()):
 nullable=t.startswith('*');t=t.lstrip('*')
 if (pkg,t) in aliases:t=aliases[pkg,t]
 if t=='supportcase.Category':
  category_source=(ROOT/'shared/go/supportcase/supportcase.go').read_text(encoding='utf-8-sig')
  out={'type':'string','enum':re.findall(r'\bCategory\s*=\s*"([A-Z_]+)"',category_source)}
 elif t in ['string','time.Time']:out={'type':'string'}
 elif t=='bool':out={'type':'boolean'}
 elif re.fullmatch(r'(u?int\d*|byte)',t):out={'type':'integer'}
 elif t in ['float32','float64']:out={'type':'number'}
 elif t.startswith('[]'):out={'type':'array','items':go_schema(pkg,t[2:],seen)}
 elif t.startswith('map['):out={'type':'object','additionalProperties':True}
 elif t in ['any','interface{}','json.RawMessage']:out={}
 else:
  s=resolve_struct(pkg,t)
  if s and not s['fields'] and pkg.endswith('/shop'):
   text=(ROOT/s['file']).read_text(encoding='utf-8-sig')
   match=re.search(r'type '+re.escape(t)+r' struct\s*\{(.*?)\}',text,re.S)
   if match:
    parsed=[]
    for line in match[1].splitlines():
     field=re.match(r'\s*(\w+)\s+([*\w\[\]]+)\s*$',line)
     if field:
      name=re.sub(r'([A-Z]+)([A-Z][a-z])',r'\1_\2',field[1])
      name=re.sub(r'([a-z0-9])([A-Z])',r'\1_\2',name).lower().replace('_i_ds','_ids')
      parsed.append({'name':name,'type':field[2],'required':False,'embedded':False})
    s={**s,'fields':parsed}
  if s and (s['package'],s['name']) not in seen:out=struct_schema(s,seen+((s['package'],s['name']),))
  else:out={}
 if nullable:out['nullable']=True
 return out

def struct_schema(s,seen=()):
 props={};required=[]
 for f in s['fields']:
  if f['embedded']:
   inner=go_schema(s['package'],f['type'],seen)
   if inner.get('type')!='object' or 'properties' not in inner: return {}
   props.update(inner['properties']);required+=inner.get('required',[])
  else:
   props[f['name']]=go_schema(s['package'],f['type'],seen)
   if f['required']:required.append(f['name'])
 return {'type':'object','properties':props,'required':required,'additionalProperties':False}

def resolve_refs(d,s,seen=()):
 if not isinstance(s,dict):return s
 if '$ref' in s:
  ref=s['$ref'];target=d
  if ref in seen:return {}
  if not ref.startswith('#/'):return {}
  for part in ref[2:].split('/'):target=target.get(part,{})
  return resolve_refs(d,target,seen+(ref,))
 out={k:([resolve_refs(d,x,seen) for x in v] if isinstance(v,list) else resolve_refs(d,v,seen) if isinstance(v,dict) else v) for k,v in s.items()}
 if 'allOf' in out and all(isinstance(x,dict) and x.get('type','object')=='object' for x in out['allOf']):
  merged={'type':'object','properties':{},'required':[]}
  for x in out.pop('allOf'):merged['properties'].update(x.get('properties',{}));merged['required']+=x.get('required',[])
  merged.update(out);out=merged
 return out

def canonical(path):return re.sub(r':(\w+)',r'{\1}',path).rstrip('/')

def camel(x):
 words=re.findall(r'[A-Za-z0-9]+',x);out=''.join(w[0].upper()+w[1:] for w in words) or 'Root'
 if out[0].isdigit():out='Value'+out
 return out

def field_name(x):
 v=camel(x);v=v[0].lower()+v[1:]
 return v+'Value' if v in {'class','final','switch','default','case','this','super','null','true','false','enum','is','as','in','return','with','static','var','new','void','required','toJson','value','hashCode','runtimeType','noSuchMethod','toString'} else v

def module(path):
 p=path.removeprefix('/api/v1/vendor/')
 if p.startswith('auth/'):return 'auth'
 if p.split('/')[0] in ['staff','roles','permissions','assignments']:return 'access'
 if p.startswith('businesses') or p=='capability-types':return 'business'
 if p.startswith('onboarding'):return 'onboarding'
 if p.startswith('media/'):return 'business'
 if p.startswith('commerce/orders'):
  if '/handover/' in p:return 'handover'
  if '/readiness' in p:return 'readiness'
  if '/support-cases' in p:return 'support'
  return 'orders'
 if p.startswith('commerce/section21'):return 'scheduled_commerce'
 if '/orchestration' in p:return 'orchestration'
 if p.startswith('fulfilment') or p.startswith('delivery/rider'):return 'delivery'
 return {'food':'food_catalogue','shop':'shop_catalogue','cloud-kitchen':'cloud_kitchen','pastry':'pastry','production':'production','production-batches':'production_batches','ingredients':'production_master','packaging-materials':'production_master','recipes':'recipes','inventory':'inventory','purchasing':'purchasing','suppliers':'purchasing','purchase-orders':'purchasing','goods-receipts':'purchasing','cost':'operational_cost','operational-cost':'operational_cost','crm':'crm','finance':'finance','taxonomy':'taxonomy','promotions':'promotions','delivery':'delivery','campaigns':'marketing'}.get(p.split('/')[0],p.split('/')[0].replace('-','_'))

# Build source route index. OpenAPI determines service public scope; source fills
# DTO gaps and can supersede stale schemas only with an actual binding.
route_index={}
for r in source['routes']:
 if 'Internal' in r['function'] or '/internal/' in r['path']:continue
 p=canonical(r['path']);p=p if p.startswith('/api/') else '/api/v1/vendor'+p
 route_index.setdefault((r['method'],p),[]).append(r)

def handler_for(method,path):
 candidates=route_index.get((method,path),[])
 if not candidates:return None,None
 r=candidates[0];m=re.search(r'\b\w+\.(\w+)',r['handler'])
 if not m:return r,None
 ff=functions.get((r['package'],m[1]),[])
 ff=[f for f in ff if 'gin.Context' in f['source']]
 ff.sort(key=lambda f: f['file']!=r['file'])
 return r,ff[0] if ff else None

def body_for(r,f):
 if not r or not f:return None,None
 compact=re.sub(r'\s+','',f['source'])
 matches=[b for b in f['bindings'] if re.search(r'(?:Decode|BindJSON|strict[A-Za-z]*|decode[A-Za-z]*)\([^)]*&'+re.escape(b['name'])+r'\b',compact)]
 # Delegating generic mutation helpers, e.g. readiness advance.
 if not matches:
  for n in re.findall(r'\bh\.(\w+)\s*\(',f['source']):
   for helper in functions.get((f['package'],n),[]):
    for b in helper['bindings']:
     if re.search(r'(?:Decode|BindJSON|strict[A-Za-z]*|decode[A-Za-z]*)\([^)]*&'+re.escape(b['name'])+r'\b',re.sub(r'\s+','',helper['source'])):matches.append(b)
 if len(matches)==1:
  b=matches[0];s=struct_schema({'package':r['package'],'fields':b.get('fields',[])}) if b.get('fields') or b['type']=='struct' else go_schema(r['package'],b['type'])
  if s.get('type')=='object' and 'properties' in s:return s,f['file']+'#'+f['name']
 return None,None

def shop_inventory_contract(method,path,parameters,schema):
 prefix='/api/v1/vendor/shop/'
 if not path.startswith(prefix) or '/inventory/' not in path:return parameters,schema,None
 tail=path.removeprefix(prefix)
 origin='services/vendor-service/internal/features/shop/inventory_http.go'
 text=(ROOT/origin).read_text(encoding='utf-8-sig')
 reads={'balances':'inventoryBalances','movements':'inventoryMovements','receipts':'listReceipts','stocktakes':'listStocktakes','alerts':'listInventoryAlerts','transfers':'listTransfers','quarantines':'listQuarantines','reorder-rules':'listReorderRules'}
 if method=='get' and re.fullmatch(r'branches/\{branchId\}/inventory/(balances|movements|receipts|stocktakes|alerts|transfers|quarantines|reorder-rules)',tail):
  handler=reads[tail.split('/')[-1]]
  match=re.search(r'func \(h \*Handler\) '+handler+r'\(.*?(?=\nfunc |\Z)',text,re.S)
  assert match, 'Recheck inventory read handler'
  queries=set(re.findall(r'c.Query\("([^"]+)"',match[0]))
  if 'page(c)' in match[0]:queries.update(['limit','offset'])
  if handler=='inventoryMovements':queries.update(['from','to'])
  for q in sorted(queries):
   if not any(x.get('in')=='query' and x.get('name')==q for x in parameters):
    parameters.append({'in':'query','name':q,'required':False,'schema':{'type':'string'},'source':origin})
 types={('put','inventory/stock-bases/{stockBasisId}/reorder-rule'):'reorderRuleCommand',
        ('post','inventory/quarantines'):'quarantineCommand',
        ('post','inventory/quarantines/{quarantineId}/resolve'):'quarantineResolutionCommand',
        ('put','branches/{branchId}/inventory/stock-bases'):'ConfigureStockBasisInput',
        ('post','inventory/transfers'):'CreateTransferInput',
        ('post','inventory/transfers/{transferId}/receive'):'transferReceiptCommand',
        ('post','inventory/receipts'):'CreateReceiptInput',('post','inventory/adjustments'):'AdjustInventoryInput',
        ('post','branches/{branchId}/inventory/stocktakes'):'stocktakeCommand',
        ('put','inventory/stocktakes/{stocktakeId}/lines/{stockBasisId}'):'stocktakeCountCommand'}
 def exact_struct(name):
  corpus='\n'.join((ROOT/('services/vendor-service/internal/features/shop/'+file)).read_text(encoding='utf-8-sig') for file in ['inventory_http.go','inventory.go','inventory_operations.go','inventory_transfers.go'])
  match=re.search(r'type '+re.escape(name)+r' struct\s*\{(.*?)\}',corpus,re.S)
  assert match, 'Recheck inventory request '+name
  if name=='transferReceiptCommand':
   assert 'Lines []ReceiveTransferLineInput' in match[1], 'Recheck transfer receipt wrapper'
   return {'type':'object','properties':{'lines':{'type':'array','items':exact_struct('ReceiveTransferLineInput')}},'required':[],'additionalProperties':False}
  props={}
  for t,key in re.findall(r'\w+\s+([*\w.\[\]]+)\s+`json:"([^",]+)[^"]*"`',match[1]):
   nullable=t.startswith('*');t=t.lstrip('*')
   if t.startswith('[]'):
    element=t[2:];out={'type':'array','items':{'type':'string'} if element=='string' else exact_struct(element)}
   else:out={'type':'string' if t in ['string','time.Time'] else 'boolean' if t=='bool' else 'integer'}
   if nullable:out['nullable']=True
   props[key]=out
  assert props, 'Inventory request has no extracted fields'
  return {'type':'object','properties':props,'required':[],'additionalProperties':False}
 if (method,tail) in types:schema=exact_struct(types[method,tail])
 if method in ['post','put']:
  parameters=[x for x in parameters if x.get('in')!='header' or x.get('name','').lower()!='idempotency-key']
  parameters.append({'in':'header','name':'Idempotency-Key','required':True,'schema':{'type':'string'},'source':origin})
 return parameters,schema,origin if (method,tail) in types else None

records={};skipped=[];docs={}
for file in sorted(DOC.glob('*.json')):
 d=json.loads(file.read_text(encoding='utf-8-sig'));docs[file.name]=d
 for path,item in d.get('paths',{}).items():
  base=(d.get('servers') or [{}])[0].get('url','')
  full=canonical(path if path.startswith('/api/') else base.rstrip('/')+path)
  for method,op in item.items():
   if method not in ['get','post','put','patch','delete']:continue
   key=(method.upper(),full)
   sec=op.get('security',d.get('security',[]));ss=json.dumps(sec).lower()
   if not full.startswith('/api/v1/vendor/') or '/internal/' in full or any(x in ss for x in ['internalservice','serviceauth','hmac','customerbearer','customerBearer'.lower()]):
    skipped.append({'method':method.upper(),'path':full,'source':file.name,'reason':'Not public Vendor staff contract'});continue
   if key in records:records[key]['sources'].append(file.name);continue
   r,f=handler_for(*key)
   rb=resolve_refs(d,op.get('requestBody',{}));schema=rb.get('content',{}).get('application/json',{}).get('schema')
   schema=resolve_refs(d,schema) if schema is not None else None
   source_schema,schema_origin=body_for(r,f)
   if source_schema is not None:schema=source_schema
   parameters=[resolve_refs(d,x) for x in item.get('parameters',[])+op.get('parameters',[])]
   if f:
    source_text=f['source']
    # Read helper query names only when called by this handler.
    for helper in re.findall(r'\b(?:h\.)?(\w+)\s*\(',source_text):
     if helper==f['name']:continue
     for x in functions.get((f['package'],helper),[]):
      if 'Query(' in x['source'] or 'DefaultQuery(' in x['source'] or 'GetHeader(' in x['source']:source_text+='\n'+x['source']
    for q in set(re.findall(r'\.(?:Query|DefaultQuery|GetQuery)\s*\(\s*"([^"]+)"',source_text)):
     if not any(x.get('in')=='query' and x.get('name')==q for x in parameters):parameters.append({'in':'query','name':q,'required':False,'schema':{'type':'string'},'source':f['file']})
    for header in set(re.findall(r'\.GetHeader\s*\(\s*"([^"]+)"',source_text)):
     if header in ['Idempotency-Key','If-Match','X-Causation-ID'] and not any(x.get('in')=='header' and x.get('name','').lower()==header.lower() for x in parameters):parameters.append({'in':'header','name':header,'required':False,'schema':{'type':'string'},'source':f['file']})
    if re.search(r'expectedVersion\s*\(',f['source']) and not any(x.get('name','').lower()=='if-match' for x in parameters):parameters.append({'in':'header','name':'If-Match','required':True,'schema':{'type':'string'},'source':f['file']})
   # Explicitly inspected authenticated order command middleware.
   if method!='get' and full.startswith('/api/v1/vendor/commerce/orders/') and '/readiness' not in full:
    if not any(x.get('name')=='Idempotency-Key' for x in parameters):parameters.append({'in':'header','name':'Idempotency-Key','required':True,'schema':{'type':'string'}})
   read_only_food_preview=method=='post' and full=='/api/v1/vendor/food/items/{itemId}/pricing/resolve'
   if read_only_food_preview:
    assert f and 'PermCatalogueRead' in f['source'] and 'GetHeader(' not in f['source'], 'Recheck the food preview authority'
    parameters=[p for p in parameters if p.get('in')!='header' or p.get('name','').lower()!='idempotency-key']
   parameters,schema,inventory_origin=shop_inventory_contract(method,full,parameters,schema)
   if inventory_origin:schema_origin=inventory_origin
   if method=='put' and full=='/api/v1/vendor/food/items/{itemId}/media':
    schema={'type': 'object', 'properties': {'media': {'type': 'array', 'maxItems': 12, 'items': {'type': 'object', 'properties': {'media_ref': {'type': 'string'}, 'is_primary': {'type': 'boolean'}}, 'required': [], 'additionalProperties': False}}, 'expected_version': {'type': 'integer'}}, 'required': [], 'additionalProperties': False}
   request_known=schema is not None or method=='get'
   if f and not re.search(r'BindJSON|Decode\(|strict[A-Za-z]*\(|decode[A-Za-z]*\(',f['source']):request_known=True
   uid=method+'_'+hashlib.sha1(full.encode()).hexdigest()[:12]
   records[key]={'id':uid,'method':method.upper(),'path':full,'source':schema_origin or file.name,'sources':[file.name],'authenticated':full not in ['/api/v1/vendor/auth/'+action for action in ['start','resend','verify','refresh','logout']],'parameters':parameters,'request_schema':schema,'request_known':request_known,'request_required':rb.get('required',False) or bool(source_schema and source_schema.get('properties')),'permission':'vendor.shop.inventory.manage_policy' if method=='put' and full=='/api/v1/vendor/shop/inventory/stock-bases/{stockBasisId}/reorder-rule' else 'vendor.shop.inventory.read' if method=='get' and full=='/api/v1/vendor/shop/branches/{branchId}/inventory/reorder-rules' else ('vendor.shop.inventory.quarantine' if method=='post' else 'vendor.shop.inventory.read') if full.startswith('/api/v1/vendor/shop/') and '/inventory/quarantines' in full else 'vendor.food.catalogue.read' if read_only_food_preview else op.get('x-permission'),'module':module(full),'handler':r,'response_contract':op.get('responses',{})}

# Source-only verified public routes, omitted from older OpenAPI snapshots.
for key,candidates in route_index.items():
 method,path=key
 if not('/readiness' in path or '/orchestration' in path) or key in records:continue
 r,f=handler_for(*key);schema,origin=body_for(r,f)
 params=[]
 if method!='GET' and not path.endswith('/start') and '/readiness' in path:params.append({'in':'header','name':'If-Match','required':True,'schema':{'type':'string'}})
 records[key]={'id':method.lower()+'_'+hashlib.sha1(path.encode()).hexdigest()[:12],'method':method,'path':path,'source':origin or r['file'],'sources':[r['file']],'authenticated':True,'parameters':params,'request_schema':schema,'request_known':True,'request_required':bool(schema and schema.get('properties')),'permission':('vendor.readiness.read' if method=='GET' else 'vendor.readiness.manage') if '/readiness' in path else ('vendor.orchestration.read' if method=='GET' else 'vendor.orchestration.override'),'module':module(path),'handler':r,'response_contract':{}}

# Per-command model generation. Raw response payloads retain unknown fields;
# fields below are derived only from schema/source, with no display defaults.
def dart_type(s):return {'string':'String','integer':'int','number':'num','boolean':'bool','array':'List<dynamic>','object':'JsonObject'}.get(s.get('type'),'Object')
def model_class(name,s,request=False):
 props=(s.get('properties') or {});required=set((s.get('required') or []));parts=[f'class {name} extends VendorPayload {{']
 if request:
  args=[];entries=[]
  for k,v in props.items():
   n=field_name(k);typ=dart_type(v);req=k in required and not v.get('nullable',False)
   args.append(('required ' if req else '')+typ+('' if req else '?')+' '+n)
   entries.append((f"'{k}': {n}" if req else f"'{k}': ?{n}"))
  parts.append(f"  {name}({{{', '.join(args)}}}) : super({{{', '.join(entries)}}});" if args else f'  {name}() : super({{}});')
 parts.append(f'  {name}.fromJson(super.json);')
 for k,v in props.items():
  n=field_name(k);typ=dart_type(v)
  if typ=='JsonObject':expr=f"value('{k}') == null ? null : jsonObject(value('{k}'))"
  elif typ=='Object':expr=f"value('{k}')"
  else:expr=f"value('{k}') as {typ}?"
  parts.append(f'  {typ}? get {n} => {expr};')
 parts+=['}',''];return '\n'.join(parts)

groups={}
for r in records.values():groups.setdefault(r['module'],[]).append(r)
for mod,ops in sorted(groups.items()):
 if selected_modules and mod not in selected_modules:continue
 ops.sort(key=lambda x:(x['path'],x['method']));folder=OUT/'lib/features'/mod
 for sub in ['data','models','state']:(folder/sub).mkdir(parents=True,exist_ok=True)
 public=[{k:v for k,v in r.items() if k not in ['module','handler','response_contract','sources']} for r in ops]
 encoded=json.dumps(public,separators=(',',':'),ensure_ascii=True)
 route_content="// GENERATED by tool/generate_contracts.py. Do not edit.\nimport 'dart:convert';\nimport '../../../core/contracts/vendor_contract.dart';\n\nfinal "+field_name(mod)+'Operations = (jsonDecode(r\'\'\''+encoded+"'''"+") as List).map((j) => VendorOperation.fromJson(jsonObject(j))).toList(growable: false);\n"
 (folder/'data'/f'{mod}_routes.dart').write_text(route_content,encoding='utf-8')
 models=["// GENERATED from actual request schemas/Go JSON fields.\nimport '../../../core/contracts/vendor_contract.dart';\n"]
 api=["// GENERATED. Methods correspond to exact public Vendor contracts.\nimport '../../../core/contracts/vendor_contract.dart';\nimport '../../../core/network/vendor_api_client.dart';\nimport '../../../core/network/vendor_command.dart';\nimport '../../../core/network/vendor_feature_api.dart';\nimport '../../../core/network/vendor_response.dart';\nimport '../models/"+mod+"_models.dart';\nimport '"+mod+"_routes.dart';\n",'class Vendor'+camel(mod)+'Api extends VendorFeatureApi {',f'  Vendor{camel(mod)}Api(VendorApiClient client) : super(client, {field_name(mod)}Operations);']
 seen=set()
 for r in ops:
  name=r['method'].lower()+camel(r['path'].removeprefix('/api/v1/vendor/'));r['dart_method']=name
  if name in seen:name+='Variant'+r['id'][-6:]
  seen.add(name);schema=r['request_schema'];model=camel(name)+'Request'
  if schema is not None and schema.get('type')=='object':models.append(model_class(model,schema,True));bodytype=model
  else:bodytype='VendorPayload'
  pp=re.findall(r'\{(\w+)\}',r['path']);args=[f'required String {field_name(p)}' for p in pp]
  args+=['Map<String,String> query = const {}','VendorCommand? command','Map<String,String> contractHeaders = const {}']
  if not r['method']=='GET':args.append(('required ' if r['request_required'] else '')+bodytype+('' if r['request_required'] else '?')+' body')
  entries=', '.join("'"+p+"': "+field_name(p) for p in pp)
  api+=['  /// Source: '+r['source'],f"  Future<VendorResponse> {name}({{{', '.join(args)}}}) => request('{r['method']}', '{r['path']}', pathParameters: {{{entries}}}, query:query, command:command, contractHeaders:contractHeaders"+(', body:body' if r['method']!='GET' else '')+');']
 api+=['}','']
 # Schema models for server-returned entities in each module's source contracts.
 for file in sorted(set(s for r in ops for s in r['sources'] if s in docs)):
  d=docs[file];prefix=camel(mod)+camel(file.split('.openapi')[0])
  for n,s in d.get('components',{}).get('schemas',{}).items():
   s=resolve_refs(d,s)
   if s.get('type')=='object' and s.get('properties'):models.append(model_class(prefix+camel(n),s))
 modeltext='\n'.join(models)
 if len(models)==1:modeltext='// No schema-defined object models in this contract family.\n'
 (folder/'models'/f'{mod}_models.dart').write_text(modeltext,encoding='utf-8')
 # Remove unused models import when there are no body DTO classes.
 text='\n'.join(api)
 if not any(r['request_schema'] is not None and r['request_schema'].get('type')=='object' and r['method']!='GET' for r in ops):text=text.replace("import '../models/"+mod+"_models.dart';\n",'')
 if not any((r['request_schema'] is None or r['request_schema'].get('type')!='object') and r['method']!='GET' for r in ops):text=text.replace("import '../../../core/contracts/vendor_contract.dart';\n",'')
 (folder/'data'/f'{mod}_api.dart').write_text(text,encoding='utf-8')
 (folder/'state'/f'{mod}_controller.dart').write_text("import '../../../core/state/vendor_feature_controller.dart';\nimport '../../../core/state/vendor_scope.dart';\nimport '../data/"+mod+"_api.dart';\n\nclass Vendor"+camel(mod)+"Controller extends VendorFeatureController {\n  Vendor"+camel(mod)+"Controller(super.scope, this.api);\n  final Vendor"+camel(mod)+"Api api;\n}\n",encoding='utf-8')

if selected_modules:
 manifest_path=OUT/'contracts/vendor_manifest.json'
 manifest=json.loads(manifest_path.read_text(encoding='utf-8'))
 updates={key:r for key,r in records.items() if r['module'] in selected_modules}
 manifest['operations']=[row for row in manifest['operations'] if row['module'] not in selected_modules]+list(updates.values())
 for mod in selected_modules:manifest['modules'][mod]=sum(row['module']==mod for row in manifest['operations'])
 manifest['unresolved_requests']=[{'method':r['method'],'path':r['path'],'source':r['source']} for r in manifest['operations'] if not r['request_known']]
 manifest_path.write_text(json.dumps(manifest,indent=2),encoding='utf-8')
 for mod in selected_modules:
  p=OUT/'lib/features'/mod/'state'/f'{mod}_controller.dart'
  p.write_text(p.read_text().replace('Vendor'+camel(mod)+'Controller','Vendor'+camel(mod)+'FeatureController').replace("import '../../../core/state/vendor_scope.dart';\n",''),encoding='utf-8')
 print('Generated: '+', '.join(sorted(selected_modules)))
 sys.exit(0)

manifest={'operations':list(records.values()),'excluded':skipped,'source_extractor':source_file.name,'modules':{m:len(o) for m,o in groups.items()},'unresolved_requests':[{'method':r['method'],'path':r['path'],'source':r['source']} for r in records.values() if not r['request_known']]}
(OUT/'contracts').mkdir(exist_ok=True)
(OUT/'contracts/vendor_manifest.json').write_text(json.dumps(manifest,indent=2),encoding='utf-8')
# Composition root and barrel. Generic load controllers are feature-specific.
imports=["import 'package:http/http.dart' as http;", "import '../core/config/vendor_app_config.dart';", "import '../core/network/vendor_api_client.dart';", "import '../core/state/vendor_scope.dart';", "import '../features/auth/data/vendor_session_store.dart';", "import '../features/auth/state/vendor_auth_controller.dart';", "import '../features/access/state/vendor_access_controller.dart';"]
def prop(mod):return mod+'Data' if mod in ['auth','access'] else field_name(mod)
for mod in sorted(groups):
 c=OUT/'lib/features'/mod/'state'/f'{mod}_controller.dart'
 c.write_text(c.read_text().replace('Vendor'+camel(mod)+'Controller','Vendor'+camel(mod)+'FeatureController').replace("import '../../../core/state/vendor_scope.dart';\n",''),encoding='utf-8')
 imports += [f"import '../features/{mod}/data/{mod}_api.dart';",f"import '../features/{mod}/state/{mod}_controller.dart';"]
app=imports+['','class VendorApp {','  VendorApp({required VendorAppConfig config, VendorSessionStore? sessions, http.Client? httpClient}) {','    client = VendorApiClient(config: config, sessions:sessions ?? SecureVendorSessionStore(), client:httpClient, onSessionChanged:(session) {','      if (session == null) { scope.clear(); access.clear(); } else { if(scope.vendorId != session.vendorId) { scope.select(vendorId:session.vendorId); } }','      auth.sessionChanged(session);','    }, onForbidden:access.recordDenial);']
for mod in sorted(groups):app += [f'    {field_name(mod)}Api = Vendor{camel(mod)}Api(client);', f'    {prop(mod)} = Vendor{camel(mod)}FeatureController(scope, {field_name(mod)}Api);']
app += ['    auth = VendorAuthController(api:authApi,client:client);','  }','  final scope = VendorScope();','  final access = VendorAccessController();','  late final VendorApiClient client;','  late final VendorAuthController auth;']
for mod in sorted(groups):app += [f'  late final Vendor{camel(mod)}Api {field_name(mod)}Api;',f'  late final Vendor{camel(mod)}FeatureController {prop(mod)};']
app += ['  Future<void> initialize() async { await auth.restore(); final session = auth.session; if(session != null && scope.vendorId != session.vendorId) { scope.select(vendorId:session.vendorId); } }','  void dispose() { auth.dispose();']
for mod in sorted(groups):app += [f'    {prop(mod)}.dispose();']
app += ['    scope.dispose(); access.dispose(); client.close();','  }','}']
(OUT/'lib/app').mkdir(exist_ok=True)
(OUT/'lib/app/vendor_app.dart').write_text('\n'.join(app)+'\n',encoding='utf-8')
exports=["export 'app/vendor_app.dart';","export 'core/config/vendor_app_config.dart';","export 'core/contracts/vendor_contract.dart';","export 'core/network/vendor_api_client.dart';","export 'core/network/vendor_command.dart';","export 'core/network/vendor_response.dart';","export 'features/auth/data/vendor_session_store.dart';","export 'features/auth/models/vendor_session.dart';","export 'features/auth/state/vendor_auth_controller.dart';","export 'features/orders/state/vendor_order_workflow.dart';","export 'features/handover/state/vendor_pickup_workflow.dart';","export 'features/finance/state/vendor_withdrawal_workflow.dart';"]
for mod in sorted(groups):exports += [f"export 'features/{mod}/data/{mod}_api.dart';",f"export 'features/{mod}/data/{mod}_routes.dart';",f"export 'features/{mod}/models/{mod}_models.dart';",f"export 'features/{mod}/state/{mod}_controller.dart';"]
(OUT/'lib/vendor.dart').write_text('\n'.join(exports)+'\n',encoding='utf-8')
print(json.dumps({'operations':len(records),'modules':manifest['modules'],'unresolved_requests':len(manifest['unresolved_requests'])},indent=2))
