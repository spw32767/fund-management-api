package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"fund-management-api/models"
)

const ScopusInsightNormalizerVersion = "core-countries-v2"

// Explicit recognized source labels. Unlisted/placeholder values remain unresolved,
// never positive foreign evidence. Expand this versioned list with reviewed labels.
var scopusCountryLabels = strings.Split("Thailand|Japan|Netherlands|Vietnam|Australia|United States|Saudi Arabia|Portugal|Indonesia|Sri Lanka|Greece|South Korea|China|United Kingdom|Canada|Malaysia|France|Mexico|Brazil|Germany|Cambodia|Sweden|Taiwan|India|Afghanistan|Italy|Lebanon|Norway|Finland|Pakistan|Jordan|Croatia|Estonia|Turkey|South Africa|United Arab Emirates|Macao|Egypt|Philippines|Tunisia|Nigeria|Yemen|Slovakia|Ethiopia|New Zealand|Luxembourg|Cameroon|Hungary|Poland|Singapore|Russia|Cyprus|Switzerland|Bangladesh|Hong Kong|Belgium|Myanmar|Laos|Nepal|Spain|Austria|Denmark|Ireland|Iceland|Israel|Iran|Iraq|Oman|Qatar|Kuwait|Bahrain|Kenya|Ghana|Uganda|Tanzania|Morocco|Algeria|Argentina|Chile|Colombia|Peru|Ecuador|Ukraine|Romania|Bulgaria|Serbia|Slovenia|Lithuania|Latvia|Czechia|Kazakhstan|Uzbekistan|Mongolia|Brunei|Bhutan|Maldives|Mauritius|Malta|Bermuda|Puerto Rico", "|")

var scopusCountryRegistry = func() map[string]string {
	m := map[string]string{}
	for _, name := range scopusCountryLabels {
		m[insightFold(name)] = name
	}
	for alias, name := range map[string]string{"viet nam": "Vietnam", "russian federation": "Russia", "republic of korea": "South Korea", "korea, republic of": "South Korea", "macao, china": "Macao", "macau": "Macao", "türkiye": "Turkey", "turkiye": "Turkey", "czech republic": "Czechia", "lao people's democratic republic": "Laos", "brunei darussalam": "Brunei", "iran, islamic republic of": "Iran", "united states of america": "United States", "usa": "United States", "uk": "United Kingdom", "kingdom of thailand": "Thailand"} {
		m[alias] = name
	}
	return m
}()

func insightFold(s string) string    { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func insightAFIDKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func canonicalInsightCountry(s string) (string, string, bool) {
	name, ok := scopusCountryRegistry[insightFold(s)]
	return insightFold(name), name, ok
}

type ScopusInsightNormalization struct {
	Affiliations         []models.ScopusDocumentAffiliation `json:"affiliations"`
	Countries            []models.ScopusDocumentCountry     `json:"countries"`
	International        *bool                              `json:"international_collaboration"`
	AffiliationsComplete bool                               `json:"affiliations_complete"`
	CountriesComplete    bool                               `json:"countries_complete"`
	Reasons              []string                           `json:"reasons"`
	ExpectedAuthors      int                                `json:"expected_authors"`
	ProvidedAuthors      int                                `json:"provided_authors"`
	MissingAuthorAFIDs   int                                `json:"missing_author_afids"`
	UnresolvedCountries  int                                `json:"unresolved_countries"`
	PayloadHash          string                             `json:"payload_hash"`
	CatalogueHash        string                             `json:"catalogue_hash"`
}

func insightHash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

// JSON scalars may use the Search API's {$: value} wrapper. Reject other shapes.
func insightScalar(v interface{}) (string, bool) {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x), true
	case json.Number:
		return x.String(), true
	case map[string]interface{}:
		// Search scalars commonly carry XML attribute metadata alongside `$`
		// (e.g. author-count's @limit and afid's @_fa).
		if value, exists := x["$"]; exists {
			for key := range x {
				if key != "$" && !strings.HasPrefix(key, "@") {
					return "", false
				}
			}
			return insightScalar(value)
		}
	}
	return "", false
}
func insightObjects(v interface{}) ([]map[string]interface{}, bool) {
	if m, ok := v.(map[string]interface{}); ok {
		return []map[string]interface{}{m}, true
	}
	a, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]map[string]interface{}, 0, len(a))
	valid := true
	for _, item := range a {
		m, ok := item.(map[string]interface{})
		if !ok {
			valid = false
			continue
		}
		out = append(out, m)
	}
	return out, valid
}
func insightAFIDs(v interface{}) ([]string, bool) {
	items, ok := v.([]interface{})
	if !ok {
		items = []interface{}{v}
	}
	ids := map[string]bool{}
	valid := true
	for _, item := range items {
		s, ok := insightScalar(item)
		if !ok || s == "" || len(s) > 32 {
			valid = false
			continue
		}
		ids[insightAFIDKey(s)] = true
	}
	out := []string{}
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, valid && len(out) > 0
}

// NormalizeScopusInsight uses only the current payload and current catalogue.
// Every payload replaces previous derived evidence, including partial payloads.
// Partial data can prove foreign presence, but can never inherit a domestic assertion.
// Catalogue is authoritative when a referenced row exists (including a cleared country).
func NormalizeScopusInsight(raw []byte, catalogue map[string]string) ScopusInsightNormalization {
	n := ScopusInsightNormalization{Affiliations: []models.ScopusDocumentAffiliation{}, Countries: []models.ScopusDocumentCountry{}, Reasons: []string{}, ExpectedAuthors: -1, PayloadHash: insightHash(raw)}
	reasons := map[string]bool{}
	reason := func(s string) { reasons[s] = true }
	// Canonical keys match catalogue lookup/trigger LOWER(TRIM(...)), without
	// trusting DB accent-insensitive equality to merge unrelated AFIDs.
	canonicalCatalogue := map[string]string{}
	conflicts := map[string]bool{}
	catalogueIDs := make([]string, 0, len(catalogue))
	for id := range catalogue {
		catalogueIDs = append(catalogueIDs, id)
	}
	sort.Strings(catalogueIDs)
	for _, id := range catalogueIDs {
		key := insightAFIDKey(id)
		value := catalogue[id]
		if previous, exists := canonicalCatalogue[key]; exists {
			oldKey, _, oldKnown := canonicalInsightCountry(previous)
			newKey, _, newKnown := canonicalInsightCountry(value)
			if oldKnown != newKnown || oldKey != newKey || (!oldKnown && insightFold(previous) != insightFold(value)) {
				conflicts[key] = true
			}
		} else {
			canonicalCatalogue[key] = value
		}
	}
	for key := range conflicts {
		canonicalCatalogue[key] = ""
	}
	catalogue = canonicalCatalogue
	finish := func() ScopusInsightNormalization {
		for r := range reasons {
			n.Reasons = append(n.Reasons, r)
		}
		sort.Strings(n.Reasons)
		return n
	}
	var root map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil || root == nil || !json.Valid(raw) {
		reason("invalid_payload")
		n.CatalogueHash = insightHash(nil)
		return finish()
	}
	affs, affShape := insightObjects(root["affiliation"])
	if !affShape || len(affs) == 0 {
		reason("missing_or_malformed_document_affiliations")
	}
	authors, authorShape := insightObjects(root["author"])
	n.ProvidedAuthors = len(authors)
	if !authorShape || len(authors) == 0 {
		reason("missing_or_malformed_authors")
	}
	count, countShape := insightScalar(root["author-count"])
	expected, err := strconv.Atoi(count)
	if !countShape || err != nil || expected <= 0 {
		reason("missing_or_invalid_author_count")
	} else {
		n.ExpectedAuthors = expected
		if expected != len(authors) {
			reason("author_count_mismatch")
		}
	}
	// Explicit incomplete/truncated indicators defeat a nominally matching count.
	for _, key := range []string{"truncated", "authors-truncated", "affiliations-truncated"} {
		if v, ok := root[key]; ok && v != false && v != "false" && v != "0" && v != json.Number("0") {
			reason("explicit_truncation")
		}
	}
	payload := map[string][]string{}
	sources := map[string]map[string]bool{}
	declared := map[string]bool{}
	countries := map[string]models.ScopusDocumentCountry{}
	add := func(id, src string) {
		if sources[id] == nil {
			sources[id] = map[string]bool{}
		}
		sources[id][src] = true
	}
	for _, a := range affs {
		id, ok := insightScalar(a["afid"])
		if !ok || id == "" || len(id) > 32 {
			reason("missing_or_invalid_document_afid")
			if value, ok := insightScalar(a["affiliation-country"]); ok {
				if key, name, ok := canonicalInsightCountry(value); ok {
					countries[key] = models.ScopusDocumentCountry{CountryKey: key, CountryName: name, Provenance: "payload"}
				}
			}
			continue
		}
		id = insightAFIDKey(id)
		declared[id] = true
		add(id, "document_payload")
		c, ok := insightScalar(a["affiliation-country"])
		if !ok && a["affiliation-country"] != nil {
			reason("malformed_payload_country")
		}
		payload[id] = append(payload[id], c)
	}
	seenAuthors := map[string]bool{}
	for _, a := range authors {
		id, ok := insightScalar(a["authid"])
		if !ok || id == "" || seenAuthors[id] {
			reason("missing_or_duplicate_author_id")
		}
		seenAuthors[id] = true
		ids, ok := insightAFIDs(a["afid"])
		if !ok {
			reason("missing_or_malformed_author_afids")
			n.MissingAuthorAFIDs++
		}
		for _, id := range ids {
			add(id, "author_payload")
			if !declared[id] {
				reason("author_afid_missing_document_affiliation")
			}
		}
	}
	ids := []string{}
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	fingerprint := []string{}
	for _, id := range ids {
		if conflicts[id] {
			reason("conflicting_catalogue_afids")
		}
		src := []string{}
		for s := range sources[id] {
			src = append(src, s)
		}
		sort.Strings(src)
		values := map[string]bool{}
		validPayload := map[string]bool{}
		for _, value := range payload[id] {
			if value != "" {
				values[value] = true
			}
			key, _, ok := canonicalInsightCountry(value)
			if ok {
				validPayload[key] = true
			} else if value != "" {
				reason("unrecognized_payload_country")
			}
		}
		original := []string{}
		for v := range values {
			original = append(original, v)
		}
		sort.Strings(original)
		b, _ := json.Marshal(original)
		n.Affiliations = append(n.Affiliations, models.ScopusDocumentAffiliation{Afid: id, Provenance: strings.Join(src, "+"), PayloadCountry: string(b)})
		catalogueValue, exists := catalogue[id]
		fingerprint = append(fingerprint, id+"\x00"+strconv.FormatBool(exists)+"\x00"+catalogueValue)
		resolved := map[string]models.ScopusDocumentCountry{}
		if exists {
			key, name, ok := canonicalInsightCountry(catalogueValue)
			if ok {
				resolved[key] = models.ScopusDocumentCountry{CountryKey: key, CountryName: name, Provenance: "catalogue"}
			}
			if !ok {
				reason("unresolved_catalogue_country")
			}
			for p := range validPayload {
				if !ok || p != key {
					reason("payload_catalogue_country_conflict")
				}
			}
		} else {
			// Unmapped AFIDs remain stored; recognized payload evidence is usable.
			for key := range validPayload {
				_, name, _ := canonicalInsightCountry(key)
				resolved[key] = models.ScopusDocumentCountry{CountryKey: key, CountryName: name, Provenance: "payload"}
			}
		}
		if len(validPayload) > 1 {
			reason("conflicting_payload_countries")
		}
		if len(resolved) == 0 {
			reason("unresolved_affiliation_country")
			n.UnresolvedCountries++
		}
		for key, c := range resolved {
			if old, ok := countries[key]; ok && old.Provenance != c.Provenance {
				c.Provenance = "payload+catalogue"
			}
			countries[key] = c
		}
	}
	fp, _ := json.Marshal(fingerprint)
	n.CatalogueHash = insightHash(fp)
	keys := []string{}
	for key := range countries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	foreign := false
	for _, key := range keys {
		n.Countries = append(n.Countries, countries[key])
		foreign = foreign || key != "thailand"
	}
	// Country conflicts also prevent full completeness even if catalogue supplies evidence.
	n.AffiliationsComplete = affShape && authorShape && len(affs) > 0 && len(authors) > 0 && n.ExpectedAuthors == len(authors) && !reasons["missing_or_invalid_document_afid"] && !reasons["missing_or_duplicate_author_id"] && !reasons["missing_or_malformed_author_afids"] && !reasons["author_afid_missing_document_affiliation"] && !reasons["explicit_truncation"]
	n.CountriesComplete = n.AffiliationsComplete && len(ids) > 0 && n.UnresolvedCountries == 0 && !reasons["conflicting_payload_countries"] && !reasons["payload_catalogue_country_conflict"] && !reasons["unrecognized_payload_country"] && !reasons["malformed_payload_country"]
	if foreign {
		v := true
		n.International = &v
	} else if n.CountriesComplete && len(keys) == 1 && keys[0] == "thailand" {
		v := false
		n.International = &v
	}
	return finish()
}
