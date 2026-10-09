"""Guarded TEST-only rollout inspection/backup/migration/reconciliation.
Never prints configuration secrets, process SQL, names, emails or raw payloads.
Schema/evidence backups use Windows DPAPI for the current user, outside Git.
No production identity is accepted; no harvesting or application startup.
"""
import argparse
import collections
import ctypes
import datetime as dt
import decimal
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import subprocess

import pymysql
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[2]
EXPECTED_DB = 'drnadech_fund_cpkku_intern'
EXPECTED_FP = '473f44f70b3c'
NEW_TABLES = ['scopus_document_affiliations', 'scopus_document_countries', 'scopus_document_insights', 'scopus_country_catalogue_guard']
BASE_TABLES = ['scopus_documents', 'scopus_document_authors', 'scopus_authors', 'scopus_affiliations', 'scopus_source_metrics', 'users']
BACKUP_ROOT = ROOT.parent / '.test-rollout-backup'
MIGRATION = ROOT / 'migrations/050_20261005_scopus_core_insights.sql'
REPORT_ROOT = ROOT / 'docs/audits'
db = None

def serialize(value):
    if isinstance(value, (dt.datetime, dt.date)): return value.isoformat()
    if isinstance(value, bytes): return value.hex()
    if isinstance(value, decimal.Decimal): return str(value)
    raise TypeError(type(value).__name__)

def encoded(value): return json.dumps(value, sort_keys=True, default=serialize).encode()
def digest(value): return hashlib.sha256(encoded(value)).hexdigest()
def query(sql, args=None):
    with db.cursor() as cur:
        try: cur.execute(sql, args)
        except pymysql.Error as exc:
            if exc.args and exc.args[0] == 1054:
                missing = re.search(r"Unknown column '([a-zA-Z0-9_.]+)'", str(exc.args[1]))
                raise RuntimeError('SQL 1054: unknown inspected column '+(missing[1] if missing else '(withheld)')) from None
            raise
        return cur.fetchall()

def connect():
    load_dotenv(ROOT / '.env', override=False)
    fp = hashlib.sha256((os.environ.get('DB_HOST', '')+':'+os.environ.get('DB_PORT', '')).encode()).hexdigest()[:12]
    if os.environ.get('ENVIRONMENT') != 'development' or os.environ.get('DB_DATABASE') != EXPECTED_DB or fp != EXPECTED_FP:
        raise RuntimeError('TARGET_AMBIGUITY: environment/database/endpoint fingerprint differs from Phase 1')
    connection = pymysql.connect(host=os.environ['DB_HOST'], port=int(os.environ.get('DB_PORT') or 3306), user=os.environ['DB_USERNAME'], password=os.environ['DB_PASSWORD'], database=EXPECTED_DB, charset='utf8mb4', connect_timeout=10, read_timeout=45, write_timeout=20, autocommit=True, cursorclass=pymysql.cursors.DictCursor)
    with connection.cursor() as cur:
        cur.execute('SELECT DATABASE() db,VERSION() version')
        identity = cur.fetchone()
    if identity['db'] != EXPECTED_DB or not identity['version'].startswith('10.11.') or 'MariaDB' not in identity['version']:
        connection.close()
        raise RuntimeError('TARGET_AMBIGUITY: connected identity/version differs from reviewed target family')
    return connection

def migration_statements():
    return [s.strip() for s in '\n'.join(line for line in MIGRATION.read_text(encoding='utf-8').splitlines() if not line.lstrip().startswith('--')).split(';') if s.strip()]

def normalized(sql): return re.sub(r'\s+', '', sql.replace('`', '')).lower()

def inventory():
    tables = query('SELECT TABLE_NAME,ENGINE,TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()')
    names = {r['TABLE_NAME'] for r in tables}
    columns = query("SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN %s ORDER BY TABLE_NAME,ORDINAL_POSITION", (BASE_TABLES+NEW_TABLES,))
    indexes = query("SELECT TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN %s ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX", (BASE_TABLES+NEW_TABLES,))
    fks = query("SELECT TABLE_NAME,CONSTRAINT_NAME,COLUMN_NAME,REFERENCED_TABLE_NAME,REFERENCED_COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL AND TABLE_NAME IN %s ORDER BY TABLE_NAME,CONSTRAINT_NAME", (BASE_TABLES+NEW_TABLES,))
    triggers = query("SELECT TRIGGER_NAME,EVENT_MANIPULATION,EVENT_OBJECT_TABLE,ACTION_TIMING,ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE IN ('scopus_documents','scopus_affiliations') ORDER BY TRIGGER_NAME")
    return names, {'tables': [r for r in tables if r['TABLE_NAME'] in BASE_TABLES+NEW_TABLES], 'columns': columns, 'indexes': indexes, 'foreign_keys': fks, 'triggers': triggers}

def fingerprints():
    # Source data is read only and never written to output; only deterministic hashes.
    result = {}
    for label, sql in {
        'documents': 'SELECT id,SHA2(raw_json,256) raw_hash,author_role_status,author_role_checked_at,updated_at FROM scopus_documents ORDER BY id',
        'author_links': 'SELECT id,document_id,author_id,affiliation_id,author_seq,is_first_author,is_corresponding_author FROM scopus_document_authors ORDER BY id',
        'catalogue': 'SELECT id,afid,country FROM scopus_affiliations ORDER BY id',
        'cohort_users': "SELECT user_id,SHA2(TRIM(scopus_id),256) scopus_id_hash,delete_at,is_test,role_id FROM users ORDER BY user_id",
        'source_metrics': 'SELECT * FROM scopus_source_metrics ORDER BY source_metric_id',
    }.items():
        result[label] = digest(query(sql))
    return result

def activity(names):
    current = query('SELECT CONNECTION_ID() id')[0]['id']
    processes = query('SHOW FULL PROCESSLIST')
    visible = [r for r in processes if r.get('db') == EXPECTED_DB and r['Id'] != current]
    writers = [r for r in visible if r.get('Command') != 'Sleep' and re.match(r'^\s*(insert|update|delete|replace|alter|create|drop|truncate|load|call)\b', r.get('Info') or '', re.I)]
    jobs = {}
    for table in sorted(names):
        if 'scopus' in table and ('run' in table or 'job' in table):
            cols = {r['COLUMN_NAME'] for r in query('SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=%s', (table,))}
            if 'status' in cols:
                jobs[table] = query('SELECT COUNT(*) n FROM `'+table+"` WHERE status IN ('running','cancelling')")[0]['n']
    job_details = {}
    recent_jobs = 0
    for table, count in jobs.items():
        if not count: continue
        cols = {r['COLUMN_NAME'] for r in query('SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=%s', (table,))}
        selected = [column for column in ['id','status','started_at','finished_at','created_at','updated_at'] if column in cols]
        job_details[table] = query('SELECT '+','.join(selected)+' FROM `'+table+"` WHERE status IN ('running','cancelling') LIMIT 20")
        for job in job_details[table]:
            stamp = job.get('updated_at') or job.get('started_at') or job.get('created_at')
            if not stamp or stamp > dt.datetime.now(dt.timezone.utc).replace(tzinfo=None)-dt.timedelta(days=1): recent_jobs += 1
    requests = None
    if 'scopus_api_requests' in names:
        requests = query('SELECT COUNT(*) count,MAX(created_at) last_created_at FROM scopus_api_requests')[0]
    return {'visible_connections': len(visible), 'active_visible_writers': len(writers), 'active_job_rows': jobs, 'active_job_details':job_details,'recent_or_undated_running_jobs':recent_jobs,'request_history':requests,'limitation': 'point-in-time own-account sessions/job records; >24h records retained without cancellation, no guarantee against new external jobs'}

def inspect():
    names, inv = inventory()
    identity = query('SELECT DATABASE() database_name,VERSION() version,@@read_only server_read_only')[0]
    grants = [next(iter(r.values())) for r in query('SHOW GRANTS')]
    target_privileges = set()
    process_visibility = False
    for grant in grants:
        m = re.match(r'GRANT (.+?) ON (.+?) TO ', grant, re.I)
        if not m: continue
        privileges, scope = m.groups()
        scope = scope.replace('`', '').replace('\\_', '_').replace('\\%', '%')
        if scope in ('*.*', EXPECTED_DB+'.*'): target_privileges.update(p.strip().upper() for p in privileges.split(','))
        if scope == '*.*' and ('PROCESS' in privileges or 'ALL PRIVILEGES' in privileges): process_visibility = True
    needed = {'SELECT','INSERT','UPDATE','DELETE','CREATE','TRIGGER'}
    permitted = 'ALL PRIVILEGES' in target_privileges or needed <= target_privileges
    state = activity(names)
    compatible_id = any(r['TABLE_NAME']=='scopus_documents' and r['COLUMN_NAME']=='id' and r['COLUMN_TYPE']=='bigint(20) unsigned' for r in inv['columns'])
    return {'timestamp_utc': dt.datetime.now(dt.timezone.utc).isoformat(), 'environment': 'development', 'database': EXPECTED_DB, 'endpoint_fingerprint_matches_phase1': True, 'identity':identity, 'migration_sha256':hashlib.sha256(MIGRATION.read_bytes()).hexdigest(), 'new_tables_present':sorted(names & set(NEW_TABLES)), 'schema':inv, 'privileges':{'required_available':permitted,'process_visibility':process_visibility,'target_privilege_codes':sorted(target_privileges)}, 'activity':state, 'document_id_compatible':compatible_id, 'documents':query('SELECT COUNT(*) count,MAX(id) max_id FROM scopus_documents')[0], 'source_fingerprints':fingerprints(), 'existing_evidence_counts':{table:query('SELECT COUNT(*) n FROM `'+table+'`')[0]['n'] for table in NEW_TABLES if table in names}}

class Blob(ctypes.Structure):
    _fields_ = [('size',ctypes.c_uint32),('data',ctypes.POINTER(ctypes.c_ubyte))]

def dpapi(data, decrypt=False):
    buffer = ctypes.create_string_buffer(data)
    source = Blob(len(data),ctypes.cast(buffer,ctypes.POINTER(ctypes.c_ubyte)))
    target = Blob()
    crypt = ctypes.WinDLL('crypt32',use_last_error=True)
    if decrypt: okay = crypt.CryptUnprotectData(ctypes.byref(source),None,None,None,None,1,ctypes.byref(target))
    else: okay = crypt.CryptProtectData(ctypes.byref(source),'TEST schema/evidence backup',None,None,None,1,ctypes.byref(target))
    if not okay: raise RuntimeError('DPAPI backup protection/recovery failed')
    try: return ctypes.string_at(target.data,target.size)
    finally: ctypes.WinDLL('kernel32').LocalFree(target.data)

def guard_activity(report):
    if not report['privileges']['required_available'] or not report['document_id_compatible']:
        raise RuntimeError('schema/privilege preflight does not permit reviewed migration')
    if report['activity']['active_visible_writers'] or report['activity']['recent_or_undated_running_jobs']:
        raise RuntimeError('active TEST writer/Scopus job requires deferring dependent writes')

def backup(report):
    guard_activity(report)
    names, _ = inventory()
    private = {'created_at_utc':report['timestamp_utc'], 'database':EXPECTED_DB, 'migration_sha256':report['migration_sha256'], 'preflight':report, 'create_tables':{}, 'create_triggers':{}, 'evidence':{}}
    for table in BASE_TABLES+NEW_TABLES:
        if table in names:
            private['create_tables'][table] = query('SHOW CREATE TABLE `'+table+'`')[0]
            if table in NEW_TABLES: private['evidence'][table] = query('SELECT * FROM `'+table+'`')
    for row in report['schema']['triggers']:
        private['create_triggers'][row['TRIGGER_NAME']] = query('SHOW CREATE TRIGGER `'+row['TRIGGER_NAME']+'`')[0]
    payload = encoded(private)
    protected = dpapi(payload)
    if dpapi(protected,True) != payload: raise RuntimeError('backup roundtrip mismatch')
    BACKUP_ROOT.mkdir(exist_ok=True)
    filename = BACKUP_ROOT / ('test-insights-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'.json.dpapi')
    filename.write_bytes(protected)
    if dpapi(filename.read_bytes(),True) != payload: raise RuntimeError('written backup recovery mismatch')
    receipt = {'backup_file':str(filename),'encrypted_sha256':hashlib.sha256(protected).hexdigest(),'recoverability':'DPAPI current Windows user; encrypt/decrypt and disk roundtrip verified','database':EXPECTED_DB,'migration_sha256':report['migration_sha256'],'schema_state_hash':digest(report['schema']),'source_fingerprints':report['source_fingerprints']}
    (REPORT_ROOT/'faculty_insights_test_backup_receipt.json').write_text(json.dumps(receipt,indent=2),encoding='utf-8')
    return receipt

def migrate(report):
    guard_activity(report)
    receipt = json.loads((REPORT_ROOT/'faculty_insights_test_backup_receipt.json').read_text())
    path = Path(receipt['backup_file']).resolve()
    if path.parent != BACKUP_ROOT.resolve() or not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest()!=receipt['encrypted_sha256']:
        raise RuntimeError('missing/mismatched protected backup')
    private = json.loads(dpapi(path.read_bytes(),True))
    if receipt['schema_state_hash']!=digest(report['schema']) or receipt['migration_sha256']!=report['migration_sha256'] or private['database']!=EXPECTED_DB:
        raise RuntimeError('schema/migration changed since backup; inspect before DDL')
    if report['source_fingerprints'] != receipt['source_fingerprints']:
        raise RuntimeError('source data changed since backup; inspect ongoing writes before DDL')
    existing = {r['TRIGGER_NAME']:r for r in report['schema']['triggers']}
    actions = []
    for sql in migration_statements():
        if sql.upper().startswith('DROP TRIGGER'):
            continue # Never drop healthy/existing triggers gratuitously.
        match = re.match(r'CREATE TRIGGER (\w+) (BEFORE|AFTER) (INSERT|UPDATE|DELETE) ON (\w+)\s+FOR EACH ROW\s+(.+)',sql,re.I|re.S)
        if match and match[1] in existing:
            name,timing,event,table,body = match.groups()
            old = existing[name]
            if (old['ACTION_TIMING'],old['EVENT_MANIPULATION'],old['EVENT_OBJECT_TABLE'])!=(timing,event,table) or normalized(old['ACTION_STATEMENT'])!=normalized(body):
                raise RuntimeError('existing trigger differs from reviewed migration: '+name)
            actions.append({'trigger':name,'action':'preserved_matching'})
            continue
        query(sql)
        actions.append({'action':'executed','object': match[1] if match else (re.search(r'CREATE TABLE IF NOT EXISTS (\w+)',sql).group(1) if sql.startswith('CREATE TABLE') else 'guard_seed')})
    return {'timestamp_utc':dt.datetime.now(dt.timezone.utc).isoformat(),'database':EXPECTED_DB,'actions':actions,'dropped_triggers':0,'after':inspect()}

def verify(report):
    if set(report['new_tables_present']) != set(NEW_TABLES): raise RuntimeError('migration tables missing')
    inv=report['schema']
    for table in NEW_TABLES:
        if not any(row['TABLE_NAME']==table and row['ENGINE']=='InnoDB' for row in inv['tables']): raise RuntimeError('new table is not InnoDB: '+table)
    expected_keys = {'scopus_document_affiliations':['document_id','afid'],'scopus_document_countries':['document_id','country_key'],'scopus_document_insights':['document_id'],'scopus_country_catalogue_guard':['id']}
    for table, key in expected_keys.items():
        actual=[r['COLUMN_NAME'] for r in inv['indexes'] if r['TABLE_NAME']==table and r['INDEX_NAME']=='PRIMARY']
        if actual!=key: raise RuntimeError('unexpected primary key: '+table)
    for table in NEW_TABLES[:3]:
        if not any(r['TABLE_NAME']==table and r['COLUMN_NAME']=='document_id' and r['REFERENCED_TABLE_NAME']=='scopus_documents' and r['REFERENCED_COLUMN_NAME']=='id' for r in inv['foreign_keys']): raise RuntimeError('document FK missing: '+table)
    delete_rules=query("SELECT TABLE_NAME,DELETE_RULE FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME IN %s",(NEW_TABLES[:3],))
    if len(delete_rules)!=3 or any(r['DELETE_RULE']!='CASCADE' for r in delete_rules): raise RuntimeError('unexpected FK delete rules')
    existing={r['TRIGGER_NAME']:r for r in inv['triggers']}
    checked=[]
    for sql in migration_statements():
        match=re.match(r'CREATE TRIGGER (\w+) (BEFORE|AFTER) (INSERT|UPDATE|DELETE) ON (\w+)\s+FOR EACH ROW\s+(.+)',sql,re.I|re.S)
        if not match: continue
        name,timing,event,table,body=match.groups();old=existing.get(name)
        if not old or (old['ACTION_TIMING'],old['EVENT_MANIPULATION'],old['EVENT_OBJECT_TABLE'])!=(timing,event,table) or normalized(old['ACTION_STATEMENT'])!=normalized(body): raise RuntimeError('required trigger mismatch: '+name)
        checked.append(name)
    guard=query('SELECT id,revision FROM scopus_country_catalogue_guard ORDER BY id')
    if len(guard)!=1 or guard[0]['id']!=1: raise RuntimeError('singleton guard missing/ambiguous')
    receipt=json.loads((REPORT_ROOT/'faculty_insights_test_backup_receipt.json').read_text())
    unchanged=report['source_fingerprints']==receipt['source_fingerprints']
    if not unchanged: raise RuntimeError('source/roles/cohort/metrics changed since pre-DDL backup')
    counts={table:query('SELECT COUNT(*) n FROM `'+table+'`')[0]['n'] for table in NEW_TABLES}
    evidence={'counts':counts,'guard':guard,'foreign_keys':delete_rules,'required_triggers_verified':checked,'source_fingerprints_preserved':unchanged}
    if counts['scopus_document_insights']:
        orders={'scopus_document_affiliations':'document_id,afid','scopus_document_countries':'document_id,country_key','scopus_document_insights':'document_id','scopus_country_catalogue_guard':'id'}
        evidence['derived_fingerprints']={table:digest(query('SELECT * FROM `'+table+'` ORDER BY '+orders[table])) for table in NEW_TABLES}
        evidence['international']=query('SELECT international_collaboration,COUNT(*) count FROM scopus_document_insights GROUP BY international_collaboration')
        evidence['statuses']=query('SELECT status,normalizer_version,COUNT(*) count FROM scopus_document_insights GROUP BY status,normalizer_version')
        evidence['completeness']=query('SELECT SUM(affiliations_complete) affiliations_complete,SUM(countries_complete) countries_complete FROM scopus_document_insights')[0]
        evidence['unknown_reasons']=query('SELECT document_id,reasons_json FROM scopus_document_insights WHERE international_collaboration IS NULL ORDER BY document_id')
        evidence['duplicate_country_keys']=query('SELECT COUNT(*) n FROM (SELECT document_id,country_key FROM scopus_document_countries GROUP BY document_id,country_key HAVING COUNT(*)>1) duplicates')[0]['n']
        evidence['coverage']=query('SELECT COUNT(*) documents,SUM(i.document_id IS NOT NULL) with_insights FROM scopus_documents d LEFT JOIN scopus_document_insights i ON i.document_id=d.id')[0]
    return {'timestamp_utc':dt.datetime.now(dt.timezone.utc).isoformat(),'database':EXPECTED_DB,'identity':report['identity'],'source_fingerprints':report['source_fingerprints'],'verification':evidence,'activity':report['activity']}

def backfill(report, apply):
    guard_activity(report);verify(report)
    receipt=json.loads((REPORT_ROOT/'faculty_insights_test_backup_receipt.json').read_text())
    private=json.loads(dpapi(Path(receipt['backup_file']).read_bytes(),True))
    through=private['preflight']['documents']['max_id']
    binary=BACKUP_ROOT/'scopus-core-insights.exe'
    if not binary.is_file(): raise RuntimeError('reviewed local backfill CLI not built')
    after=0;batches=[]
    if apply:
        dry=json.loads((REPORT_ROOT/'faculty_insights_test_dry_run.json').read_text())
        if dry['through_id']!=through or dry['processed']!=private['preflight']['documents']['count']: raise RuntimeError('missing/mismatched complete bounded dry-run')
    while after<through:
        # Fresh identity/source/job guard before every bounded invocation.
        guard_activity(inspect())
        if fingerprints()!=receipt['source_fingerprints']: raise RuntimeError('concurrent source change; stop/resume after review')
        args=[str(binary),'--expect-database',EXPECTED_DB,'--through-id',str(through),'--after-id',str(after),'--limit',('100' if apply else '500'),'--batch-size','50']
        if apply: args.append('--apply')
        child=subprocess.run(args,cwd=ROOT,env=os.environ.copy(),capture_output=True,text=True,timeout=180)
        try: result=json.loads(child.stdout)
        except ValueError: raise RuntimeError('CLI returned no sanitized JSON result') from None
        batches.append(result)
        progress={'timestamp_utc':dt.datetime.now(dt.timezone.utc).isoformat(),'database':EXPECTED_DB,'through_id':through,'batches':batches,'binary_sha256':hashlib.sha256(binary.read_bytes()).hexdigest(),'processed':sum(b['processed'] for b in batches),'last_id':result['last_id'],'errors':int(child.returncode!=0)}
        (REPORT_ROOT/('faculty_insights_test_'+('apply' if apply else 'dry_run')+'.json')).write_text(json.dumps(progress,indent=2),encoding='utf-8')
        if child.returncode: raise RuntimeError('CLI failed; sanitized cursor saved; no automatic retry')
        if result['last_id']<=after: break
        after=result['last_id']
    totals=collections.Counter()
    reasons=collections.Counter()
    for batch in batches:
        totals.update({k:batch[k] for k in ['processed','yes','no','unknown','complete']});reasons.update(batch['reasons'])
    progress['totals']=dict(totals);progress['reasons']=dict(reasons)
    if progress['processed']!=private['preflight']['documents']['count']: raise RuntimeError('backfill coverage differs from fixed preflight population')
    return progress

def main():
    global db
    p=argparse.ArgumentParser();p.add_argument('action',choices=['inspect','backup','migrate','verify','dry_run','apply']);args=p.parse_args()
    db=connect()
    if args.action not in ('migrate','apply'):
        query('SET SESSION TRANSACTION READ ONLY')
    report=inspect()
    if args.action=='backup': result=backup(report)
    elif args.action=='migrate': result=migrate(report)
    elif args.action=='verify': result=verify(report)
    elif args.action in ('dry_run','apply'): result=backfill(report,args.action=='apply')
    else: result=report
    output=REPORT_ROOT/('faculty_insights_test_'+args.action+'.json')
    output.write_text(json.dumps(result,indent=2,default=serialize),encoding='utf-8')
    print(json.dumps({'action':args.action,'database':EXPECTED_DB,'evidence_file':output.name,'documents':report['documents'],'new_tables_present':report['new_tables_present'],'required_privileges':report['privileges']['required_available'],'active_writers':report['activity']['active_visible_writers'],'active_jobs':report['activity']['active_job_rows']},default=serialize))

if __name__=='__main__':
    try: main()
    except Exception as exc:
        code=exc.args[0] if exc.args and isinstance(exc.args[0],int) else None
        detail=str(exc) if isinstance(exc,RuntimeError) else 'Database/runtime error withheld to avoid connection identity or source data'
        print(json.dumps({'failed':True,'type':type(exc).__name__,'code':code,'detail':detail}));sys.exit(1)
    finally:
        if db: db.rollback();db.close()
