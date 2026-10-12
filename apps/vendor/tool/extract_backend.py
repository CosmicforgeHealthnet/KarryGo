from pathlib import Path
import re,json
root=Path(__file__).resolve().parents[3]
base=root/'services/vendor-service/internal/features'
result={'structs':[],'functions':[],'routes':[],'aliases':[]}
# Balanced braces ignore comments and Go string/rune literals.
lex=re.compile(r'//[^\n]*|/\*[\s\S]*?\*/|`[^`]*`|"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\'|[{}]')
def pairs(s):
 stack=[];out={}
 for m in lex.finditer(s):
  if m.group()=='{':stack.append(m.start())
  elif m.group()=='}' and stack:out[stack.pop()]=m.start()
 return out
def fields(s, untagged=False):
 out=[]
 for line in s.splitlines():
  m=re.match(r'\s*(\w+)\s+(.+?)\s+`([^`]+)`',line)
  if m:
   tag=re.search(r'json:"([^",]+)',m[3])
   if tag and tag[1]!='-':out.append({'name':tag[1],'type':m[2].strip(),'required':bool(re.search(r'binding:"[^"]*required',m[3])),'embedded':False})
  elif untagged and re.match(r'^\s*[A-Z]\w*\s+[*\w\[\].]+(?:\s*//.*)?\s*$',line):
   field=re.match(r'^\s*([A-Z]\w*)\s+([*\w\[\].]+)',line)
   out.append({'name':field[1],'type':field[2],'required':False,'embedded':False})
  elif untagged and re.match(r'^\s*[A-Z]\w*\s+map\[string\]any\s*$',line):
   field=re.match(r'^\s*([A-Z]\w*)\s+(map\[string\]any)',line)
   out.append({'name':field[1],'type':field[2],'required':False,'embedded':False})
  elif re.match(r'^\s*[\w.]+\s*$',line) and line.strip() not in ['','struct']:
   out.append({'name':'','type':line.strip(),'required':False,'embedded':True})
 return out
for p in sorted(base.rglob('*.go')):
 if p.name.endswith('_test.go'):continue
 s=p.read_text(encoding='utf-8-sig'); bp=pairs(s);pkg=p.parent.relative_to(root).as_posix();fp=p.relative_to(root).as_posix()
 for alias in re.finditer(r'^type\s+(\w+)\s+(?:=\s*)?(?!struct)([\w.*\[\]]+)\s*$',s,re.M):result['aliases'].append({'package':pkg,'name':alias[1],'type':alias[2]})
 for m in re.finditer(r'\btype\s+(\w+)\s+struct\s*{',s):
  a=m.end()-1;b=bp.get(a)
  if b is not None:result['structs'].append({'package':pkg,'name':m[1],'file':fp,'fields':fields(s[a+1:b],untagged=m[1] in ['ScheduledOrderInput','RecurringScheduleInput'])})
 for m in re.finditer(r'\bfunc\s*(?:\([^)]*\)\s*)?(\w+)\s*\(',s):
  a=s.find('{',m.end());b=bp.get(a)
  if b is None:continue
  src=s[m.start():b+1];bind=[]
  for v in re.finditer(r'\bvar\s+(\w+)\s+([^\n]+)',src):
   t=v[2].strip();q={'name':v[1],'type':t,'fields':[]}
   if t.startswith('struct'):
    sa=src.find('{',v.start());sb=pairs(src).get(sa)
    if sb is not None:q['fields']=fields(src[sa+1:sb]);q['type']='struct'
   bind.append(q)
  result['functions'].append({'package':pkg,'file':fp,'name':m[1],'source':src,'bindings':bind})
  groups={}
  for route in re.finditer(r'(\w+)\.(GET|POST|PUT|PATCH|DELETE)\s*\(\s*"([^"]*)"\s*,\s*([^\n]+)',src):
   before=src[:route.start()]
   for g in re.finditer(r'(\w+)\s*:?=\s*(\w+)\.Group\s*\(\s*"([^"]*)"',before):groups[g[1]]=groups.get(g[2],'')+g[3]
   handler=route[4].strip().rstrip(',')
   result['routes'].append({'package':pkg,'file':fp,'function':m[1],'method':route[2],'path':groups.get(route[1],'')+route[3],'handler':handler})
(root/'apps/vendor/tool/backend_ast_python.json').write_text(json.dumps(result,indent=2),encoding='utf-8')
print('Source scan:',len(result['structs']),'structs,',len(result['functions']),'functions,',len(result['routes']),'literal routes')
