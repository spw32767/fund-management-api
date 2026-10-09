"""Read-only local-config audit. Run from backend root; prints no connection secrets.
No API requests, application imports, DB mutations, or raw payload output.
Requires existing Python pymysql and python-dotenv packages.
"""
import collections as C
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import sys
import pymysql
from dotenv import load_dotenv

ROOT = Path(__file__).resolve().parents[2]
load_dotenv(ROOT / '.env', override=False)
env = {k: os.environ.get(k, '') for k in ['DB_HOST', 'DB_PORT', 'DB_DATABASE', 'DB_USERNAME', 'DB_PASSWORD', 'ENVIRONMENT']}
out = {'audited_at_utc': dt.datetime.now(dt.timezone.utc).isoformat(), 'config_source': 'backend .env with existing process environment taking precedence', 'environment_label': env['ENVIRONMENT'] or '(unset)', 'database': env['DB_DATABASE'], 'endpoint_is_loopback': env['DB_HOST'].lower() in ['localhost', '127.0.0.1', '::1'], 'endpoint_fingerprint': hashlib.sha256((env['DB_HOST']+':'+env['DB_PORT']).encode()).hexdigest()[:12], 'queries': {}}
try:
    db = pymysql.connect(host=env['DB_HOST'], port=int(env['DB_PORT'] or 3306), user=env['DB_USERNAME'], password=env['DB_PASSWORD'], database=env['DB_DATABASE'], charset='utf8mb4', connect_timeout=10, read_timeout=45, write_timeout=10, autocommit=False, cursorclass=pymysql.cursors.DictCursor)
except Exception as exc:
    out['blocker'] = {'type': type(exc).__name__, 'code': exc.args[0] if exc.args and isinstance(exc.args[0], int) else None, 'detail': 'Configured endpoint connection failed; original exception intentionally withheld because it can contain connection identity.'}
    print(json.dumps(out, indent=2)); sys.exit(2)

def query(key, sql):
    out['queries'][key] = sql
    with db.cursor() as cur:
        cur.execute(sql)
        return cur.fetchall()

def arr(x):
    return x if isinstance(x, list) else [x] if isinstance(x, dict) else []

def scalar(x):
    if isinstance(x, dict): return scalar(x.get('$'))
    return str(x).strip() if x is not None else ''

def afids(x):
    return [scalar(y) for y in (x if isinstance(x, list) else [x]) if scalar(y)]

def country(x):
    return ' '.join(scalar(x).split()).casefold()

try:
    with db.cursor() as cur:
        cur.execute('SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ')
        cur.execute('SET SESSION TRANSACTION READ ONLY')
        cur.execute('START TRANSACTION WITH CONSISTENT SNAPSHOT')
    out['transaction'] = 'REPEATABLE READ; READ ONLY; consistent snapshot; ROLLBACK on exit'
    out['server'] = query('identity', 'SELECT DATABASE() AS database_name, VERSION() AS version, @@session.tx_read_only AS transaction_read_only')
    total = query('size', 'SELECT COUNT(*) AS n FROM scopus_documents')[0]['n']
    if total > 100000: raise RuntimeError('Safety cap exceeded: more than 100000 core documents')
    out['schema'] = query('schema', "SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('scopus_documents','scopus_document_authors','scopus_authors','scopus_affiliations','scopus_source_metrics') ORDER BY TABLE_NAME,ORDINAL_POSITION LIMIT 300")
    out['indexes'] = query('indexes', "SELECT TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('users','scopus_documents','scopus_document_authors','scopus_authors','scopus_affiliations','scopus_source_metrics') ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX LIMIT 300")
    cohort = "EXISTS (SELECT 1 FROM scopus_document_authors sda JOIN scopus_authors sa ON sa.id=sda.author_id JOIN users u ON TRIM(u.scopus_id)=sa.scopus_author_id JOIN scopus_affiliations aff ON aff.id=sda.affiliation_id WHERE sda.document_id=sd.id AND u.delete_at IS NULL AND u.is_test=0 AND u.scopus_id IS NOT NULL AND TRIM(u.scopus_id)<>'' AND LOWER(TRIM(COALESCE(aff.name,''))) IN ('khon kaen university','faculty of science, khon kaen university'))"
    sql = 'SELECT sd.id,sd.eid,sd.scopus_id,sd.raw_json,sd.author_role_status,sd.author_role_checked_at,COALESCE(YEAR(sd.cover_date),CAST(RIGHT(sd.cover_display_date,4) AS UNSIGNED)) AS year_ce,('+cohort+') AS faculty FROM scopus_documents sd ORDER BY sd.id LIMIT 100000'
    docs = query('documents', sql)
    links = query('links', "SELECT sda.document_id,sa.scopus_author_id,sda.author_seq,sda.is_first_author,sda.is_corresponding_author,aff.afid,aff.country, (EXISTS(SELECT 1 FROM users u WHERE TRIM(u.scopus_id)=sa.scopus_author_id AND u.delete_at IS NULL AND u.is_test=0 AND TRIM(u.scopus_id)<>'')) AS registered, (LOWER(TRIM(COALESCE(aff.name,''))) IN ('khon kaen university','faculty of science, khon kaen university')) AS kku FROM scopus_document_authors sda JOIN scopus_authors sa ON sa.id=sda.author_id LEFT JOIN scopus_affiliations aff ON aff.id=sda.affiliation_id ORDER BY sda.document_id,sda.author_seq LIMIT 1000000")
    if len(links) == 1000000: raise RuntimeError('Link safety cap reached')
    catalog = query('catalogue', 'SELECT afid,country,name FROM scopus_affiliations ORDER BY afid LIMIT 100000')
    if len(catalog) == 100000: raise RuntimeError('Catalogue safety cap reached')
    cat = {scalar(a['afid']): country(a['country']) for a in catalog}
    kku_afids = {scalar(a['afid']) for a in catalog if country(a['name']) in ['khon kaen university','faculty of science, khon kaen university']}
    bydoc = C.defaultdict(list)
    for link in links: bydoc[link['document_id']].append(link)
    groups = {k:C.Counter() for k in ['all_core','all_core_2567_2569','faculty_all_years','faculty_2567_2569']}
    years = C.defaultdict(C.Counter)
    examples = C.defaultdict(list)
    spellings = C.Counter()
    catalog_spellings = C.Counter(scalar(a['country']) for a in catalog)
    missing_catalog = C.Counter()
    missing_country = C.Counter()
    partners = {k:C.Counter() for k in groups}
    cross = {k:C.Counter() for k in groups}
    payload_keys = C.Counter()
    def example(key,d,**extra):
        if len(examples[key]) < 8: examples[key].append({'document_id':d['id'],'eid':d['eid'],'scopus_id':d['scopus_id'],'year_ce':d['year_ce'],**extra})
    for d in docs:
        counts = C.Counter(documents=1)
        if not d['year_ce']: counts['publication_year_missing_or_zero']+=1
        raw = d['raw_json']
        payload = None
        if not raw: counts['raw_missing']+=1
        else:
            try:
                payload = json.loads(raw)
                if not isinstance(payload,dict): raise ValueError('Non-object entry')
                counts['raw_valid_object']+=1
            except (ValueError,TypeError): counts['raw_invalid_or_nonobject']+=1; example('raw_invalid',d)
        allcountries=set(); payloadcountries=set(); unionaf=set(); complete=False
        if payload is not None:
            payload_keys.update(payload.keys())
            da=arr(payload.get('affiliation')); authors=arr(payload.get('author'))
            stored_ids={scalar(l['scopus_author_id']) for l in bydoc[d['id']]}
            payload_ids={scalar(a.get('authid')) for a in authors if scalar(a.get('authid'))}
            if stored_ids!=payload_ids: counts['stored_payload_author_roster_mismatch']+=1; example('stored_payload_roster_mismatch',d,stored_only=sorted(stored_ids-payload_ids),payload_only=sorted(payload_ids-stored_ids))
            if 'affiliation' not in payload or payload['affiliation'] is None: counts['document_affiliation_absent_or_null']+=1
            elif not da: counts['document_affiliation_empty_or_bad_shape']+=1
            else: counts['document_affiliation_nonempty']+=1
            known_count=False; roster_match=False
            try:
                n=int(scalar(payload.get('author-count'))); known_count=True; roster_match=n==len(authors)
                if n>len(authors): counts['author_roster_truncated']+=1; example('truncated',d,declared=n,provided=len(authors))
                elif n!=len(authors): counts['author_count_mismatch_other']+=1
            except ValueError: counts['author_count_missing_or_invalid']+=1
            if roster_match: counts['author_count_matches']+=1
            if not authors: counts['authors_absent_or_empty']+=1
            author_af_complete=bool(authors)
            declared={scalar(a.get('afid')) for a in da if scalar(a.get('afid'))}
            for a in da:
                af=scalar(a.get('afid')); c=country(a.get('affiliation-country'))
                if c: payloadcountries.add(c); spellings[scalar(a.get('affiliation-country'))]+=1
                if not af: counts['document_affiliation_rows_missing_afid']+=1
                else: unionaf.add(af)
            for a in authors:
                ids=afids(a.get('afid'))
                if not ids: author_af_complete=False; counts['author_rows_missing_afid']+=1
                if not ids: example('missing_author_afid',d,author_id=scalar(a.get('authid')))
                if len(set(ids))>1:
                    counts['multi_afid_author_rows']+=1
                    example('multi_afid',d,author_id=scalar(a.get('authid')),afids=ids)
                    if any(cat.get(af) and cat[af]!='thailand' for af in ids) and any(cat.get(af)=='thailand' for af in ids):
                        counts['dual_domestic_foreign_author_rows']+=1
                        example('dual_domestic_foreign',d,author_id=scalar(a.get('authid')),afids=ids,countries=[cat.get(af) for af in ids])
                unionaf.update(ids)
            counts['document_afid_rows']=len(declared)
            counts['all_payload_afid_rows']=len(unionaf)
            undeclared=unionaf-declared
            if undeclared: counts['author_afid_missing_document_catalogue']+=1; example('undeclared_afids',d,afids=sorted(undeclared))
            allcountries.update(payloadcountries)
            unresolved=[]
            payloadmap={scalar(a.get('afid')):country(a.get('affiliation-country')) for a in da if scalar(a.get('afid'))}
            for af in unionaf:
                if af not in cat: missing_catalog[af]+=1; counts['afid_missing_catalogue_occurrences']+=1
                c=payloadmap.get(af) or cat.get(af)
                if payloadmap.get(af) and cat.get(af) and payloadmap[af]!=cat[af]: counts['payload_catalogue_country_conflicts']+=1; example('country_conflict',d,afid=af,payload_country=payloadmap[af],catalogue_country=cat[af])
                if c: allcountries.add(c)
                else: missing_country[af]+=1; unresolved.append(af)
                if not cat.get(af): counts['afid_catalogue_country_missing_occurrences']+=1
            if unresolved: counts['unresolved_country_documents']+=1; example('unresolved_country',d,afids=unresolved)
            registered={scalar(l['scopus_author_id']) for l in bydoc[d['id']] if l['registered']}
            all_aff_cohort=any(scalar(a.get('authid')) in registered and any(af in kku_afids for af in afids(a.get('afid'))) for a in authors)
            counts['all_affiliation_faculty_cohort']+=int(all_aff_cohort)
            if all_aff_cohort and not d['faculty']: counts['additional_all_affiliation_faculty_documents']+=1; example('additional_all_affiliation_faculty',d)
            # Conservative operational completeness; present field alone is insufficient.
            complete=bool(da) and known_count and roster_match and author_af_complete and not undeclared and not unresolved and all(bool(country(a.get('affiliation-country')) or cat.get(scalar(a.get('afid')))) and bool(scalar(a.get('afid'))) for a in da)
        legacy={country(l['country']) for l in bydoc[d['id']] if country(l['country'])}
        foreign=any(c!='thailand' for c in allcountries)
        legacy_foreign=any(c!='thailand' for c in legacy)
        status='yes' if foreign else 'no' if complete and allcountries=={'thailand'} else 'unknown'
        counts['country_'+status]+=1
        counts['affiliation_metadata_complete' if complete else 'affiliation_metadata_incomplete']+=1
        if counts['multi_afid_author_rows']: counts['multi_afid_documents']+=1
        if counts['dual_domestic_foreign_author_rows']: counts['dual_domestic_foreign_documents']+=1
        counts['legacy_international']+=int(legacy_foreign)
        counts['payload_only_international']+=int(any(c!='thailand' for c in payloadcountries))
        if foreign and not legacy_foreign: counts['international_missed_by_legacy']+=1; example('international_missed_by_legacy',d,countries=sorted(allcountries),legacy_countries=sorted(legacy))
        if legacy_foreign and not foreign: counts['legacy_foreign_without_payload_evidence']+=1; example('legacy_only_foreign',d,countries=sorted(allcountries),legacy_countries=sorted(legacy))
        counts['role_status_'+str(d['author_role_status'] or 'pending')]+=1
        eligible=[l for l in bydoc[d['id']] if l['registered'] and l['kku']]
        for prefix, subset in [('all_author',bydoc[d['id']]),('eligible_faculty_author',eligible)]:
            counts[prefix+'_rows']+=len(subset)
            counts[prefix+'_first']+=sum(l['is_first_author']==1 for l in subset)
            counts[prefix+'_corresponding']+=sum(l['is_corresponding_author']==1 for l in subset)
            counts[prefix+'_both']+=sum(l['is_first_author']==1 and l['is_corresponding_author']==1 for l in subset)
            counts[prefix+'_coauthor']+=sum(l['is_first_author']==0 and l['is_corresponding_author']==0 for l in subset)
            counts[prefix+'_null_flags']+=sum(l['is_first_author'] is None or l['is_corresponding_author'] is None for l in subset)
        first=any(l['is_first_author']==1 for l in eligible)
        corr=any(l['is_corresponding_author']==1 for l in eligible)
        co=any(l['is_first_author']==0 and l['is_corresponding_author']==0 for l in eligible)
        # First positive wins; unknown prevents lower priority classification.
        known=d['author_role_status'] in ['complete','no_correspondence']
        unknown=not known or any(l['is_first_author'] is None or l['is_corresponding_author'] is None for l in eligible)
        role='first' if first else 'unknown' if unknown or not eligible else 'corresponding' if corr else 'coauthor'
        counts['faculty_role_'+role]+=1
        counts['faculty_positive_first']+=int(first); counts['faculty_positive_corresponding']+=int(corr); counts['faculty_positive_coauthor']+=int(co)
        if first and corr: counts['faculty_first_corresponding_overlap']+=1; example('first_corresponding_overlap',d)
        if first and corr and co: counts['faculty_first_corresponding_coauthor_overlap']+=1; example('all_three_role_overlap',d)
        if any(l['is_first_author']==1 and l['is_corresponding_author']==1 for l in eligible): counts['faculty_same_author_first_corresponding']+=1; example('same_author_first_corresponding',d)
        if d['faculty'] and role=='unknown': example('faculty_unknown_role',d,role_status=d['author_role_status'])
        if status=='unknown': example('core_unknown_country',d,countries=sorted(allcountries))
        if d['faculty'] and status=='unknown': example('faculty_unknown_country',d,countries=sorted(allcountries))
        if first and corr and co:
            example('eligible_role_evidence',d,authors=[{'scopus_author_id':l['scopus_author_id'],'first':l['is_first_author'],'corresponding':l['is_corresponding_author']} for l in eligible])
        keys=['all_core']
        if d['year_ce'] and 2024<=d['year_ce']<=2026: keys.append('all_core_2567_2569')
        if d['faculty']:
            keys.append('faculty_all_years')
            years[d['year_ce']].update(counts)
            if d['year_ce'] and 2024<=d['year_ce']<=2026: keys.append('faculty_2567_2569')
        for key in keys:
            groups[key].update(counts)
            partners[key].update(c for c in allcountries if c!='thailand')
            cross[key][status+'_'+role]+=1
    out['groups']=groups
    out['faculty_years']={str(y):v for y,v in sorted(years.items(),key=lambda i:i[0] or 0)}
    out['cross_tabs']=cross
    out['partners']=partners
    out['examples']=examples
    out['payload_country_spellings']=spellings
    out['catalogue_country_spellings']=catalog_spellings
    out['missing_catalogue_afids']=missing_catalog
    out['unresolved_country_afids']=missing_country
    out['payload_top_level_key_counts']=payload_keys
    out['cohort_diagnostics']=query('cohort_diagnostics', "SELECT COUNT(*) AS users_active_non_test, SUM(role_id IN (1,4,5)) AS faculty_roster_users, SUM(scopus_id IS NOT NULL AND TRIM(scopus_id)<>'') AS users_with_scopus_id, SUM(scopus_id IS NOT NULL AND TRIM(scopus_id)<>'' AND role_id NOT IN (1,4,5)) AS non_roster_users_with_scopus_id FROM users WHERE delete_at IS NULL AND is_test=0")
    out['faculty_employment_diagnostics']=query('employment', "SELECT COUNT(DISTINCT sda.document_id) AS cohort_documents,COUNT(DISTINCT CASE WHEN u.date_of_employment IS NOT NULL AND sd.cover_date < DATE(u.date_of_employment) THEN sd.id END) AS documents_with_preemployment_eligible_author,COUNT(DISTINCT CASE WHEN u.role_id NOT IN (1,4,5) THEN sd.id END) AS documents_with_non_roster_eligible_user FROM scopus_documents sd JOIN scopus_document_authors sda ON sda.document_id=sd.id JOIN scopus_authors sa ON sa.id=sda.author_id JOIN users u ON TRIM(u.scopus_id)=sa.scopus_author_id JOIN scopus_affiliations aff ON aff.id=sda.affiliation_id WHERE u.delete_at IS NULL AND u.is_test=0 AND TRIM(u.scopus_id)<>'' AND LOWER(TRIM(COALESCE(aff.name,''))) IN ('khon kaen university','faculty of science, khon kaen university')")
    out['plan']=query('explain_cohort', 'EXPLAIN SELECT sd.id FROM scopus_documents sd WHERE '+cohort+' AND COALESCE(YEAR(sd.cover_date),CAST(RIGHT(sd.cover_display_date,4) AS UNSIGNED)) BETWEEN 2024 AND 2026')
    out['bounds']={'core_count':total,'documents_read':len(docs),'links_read':len(links),'catalogue_read':len(catalog)}
    print(json.dumps(out,indent=2,default=str,ensure_ascii=False))
finally:
    db.rollback()
    db.close()
