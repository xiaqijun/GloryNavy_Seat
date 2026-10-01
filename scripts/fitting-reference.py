"""Generate the server-side EFT and skill prerequisite reference from CCP JSONL SDE."""
import argparse,json,zipfile
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('archive',type=Path);p.add_argument('--build',required=True,type=int);a=p.parse_args()
with zipfile.ZipFile(a.archive) as z:
 groups={r['_key']:r for r in map(json.loads,z.read('groups.jsonl').splitlines())}
 attributes={r['name']:r['_key'] for r in map(json.loads,z.read('dogmaAttributes.jsonl').splitlines())}
 effects={r['_key']:r['name'] for r in map(json.loads,z.read('dogmaEffects.jsonl').splitlines())}
 dogma={r['_key']:r for r in map(json.loads,z.read('typeDogma.jsonl').splitlines())}
 slots={'loPower':'low','medPower':'medium','hiPower':'high','rigSlot':'rig','subSystem':'subsystem','serviceSlot':'service'}
 types=[]
 for r in map(json.loads,z.read('types.jsonl').splitlines()):
  g=groups[r['groupID']];cat=g['categoryID']
  if not r.get('published') or cat not in (6,7,8,16,18,20,32,65,66,87):continue
  d=dogma.get(r['_key'],{});attrs={i['attributeID']:i['value'] for i in d.get('dogmaAttributes',[])}
  slot=next((slots[effects[e['effectID']]] for e in d.get('dogmaEffects',[]) if effects.get(e['effectID']) in slots),'')
  if cat==18:slot='drone_bay'
  if cat==87:slot='fighter_bay'
  req=[]
  for i in range(1,7):
   skill=int(attrs.get(attributes[f'requiredSkill{i}'],0));level=int(attrs.get(attributes[f'requiredSkill{i}Level'],0))
   if skill: req.append({'skill_id':str(skill),'level':level})
  types.append({'id':str(r['_key']),'name':r['name'].get('zh',r['name']['en']),'english':r['name']['en'],'category':cat,'group':g['name'].get('zh',g['name']['en']),'group_english':g['name']['en'],'slot':slot,'requirements':req})
out=Path(__file__).resolve().parents[1]/'internal/modules/fittings/reference.json'
out.write_text(json.dumps({'build':a.build,'types':types},ensure_ascii=False,separators=(',',':')),encoding='utf-8')
print(f'Wrote {len(types)} types for SDE {a.build} ({out.stat().st_size} bytes)')
