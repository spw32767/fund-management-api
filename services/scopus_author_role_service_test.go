package services

import "testing"

const roleXMLPrefix = `<abstracts-retrieval-response xmlns:dc="http://purl.org/dc/elements/1.1/"><coredata><dc:creator><author auid="11"/></dc:creator></coredata><item><bibrecord><head><author-group><author auid="11" seq="1"><given-name>First</given-name><surname>Writer</surname></author><author auid="22" seq="2"><given-name>Second</given-name><surname>Writer</surname></author><author auid="33" seq="3"><given-name>Third</given-name><surname>Writer</surname></author></author-group>`
const roleXMLSuffix = `</head><tail><reference><author seq="1"><given-name>Unrelated</given-name><surname>Reference</surname></author></reference></tail></bibrecord></item></abstracts-retrieval-response>`

func sampleRoleLinks() []roleLink {
	return []roleLink{
		{LinkID: 1, AuthorID: "11", GivenName: "First", Surname: "Writer"},
		{LinkID: 2, AuthorID: "22", GivenName: "Second", Surname: "Writer"},
		{LinkID: 3, AuthorID: "33", GivenName: "Third", Surname: "Writer"},
	}
}

func TestXMLRolesFirstAndCorrespondingOverlap(t *testing.T) {
	body := roleXMLPrefix + `<correspondence><person author-instance-id="different"><given-name>First</given-name><surname>Writer</surname></person></correspondence>` + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Authors) != 3 {
		t.Fatalf("expected three document authors, got %d", len(parsed.Authors))
	}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks())
	if !ok || !roles[1].First || !roles[1].Corresponding || roles[2].First || roles[2].Corresponding {
		t.Fatalf("unexpected role resolution: ok=%v roles=%+v", ok, roles)
	}
}

func TestXMLRolesMultipleCorresponding(t *testing.T) {
	body := roleXMLPrefix + `<correspondence><person><given-name>Second</given-name><surname>Writer</surname></person></correspondence><correspondence><person><given-name>Third</given-name><surname>Writer</surname></person></correspondence>` + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks())
	if !ok || !roles[1].First || roles[1].Corresponding || !roles[2].Corresponding || !roles[3].Corresponding {
		t.Fatalf("unexpected multiple-correspondence resolution: ok=%v roles=%+v", ok, roles)
	}
}

func TestXMLRolesMultiplePeopleInOneCorrespondenceBlock(t *testing.T) {
	body := roleXMLPrefix + `<correspondence><person><given-name>Second</given-name><surname>Writer</surname></person><person><given-name>Third</given-name><surname>Writer</surname></person></correspondence>` + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks())
	if !ok || !roles[2].Corresponding || !roles[3].Corresponding {
		t.Fatalf("expected both correspondence people: ok=%v roles=%+v", ok, roles)
	}
}

func TestXMLRolesMissingCorrespondenceDefaultsToCoAuthor(t *testing.T) {
	parsed, err := parseAuthorRoleXML([]byte(roleXMLPrefix + roleXMLSuffix))
	if err != nil {
		t.Fatal(err)
	}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks())
	if !ok || len(parsed.Corresponding) != 0 || !roles[1].First || roles[2].First || roles[2].Corresponding || roles[3].First || roles[3].Corresponding {
		t.Fatalf("unexpected no-correspondence resolution: ok=%v roles=%+v", ok, roles)
	}
}

func TestXMLRolesCorrespondenceAffiliationWithoutPerson(t *testing.T) {
	body := roleXMLPrefix + `<correspondence><affiliation><organization>KKU</organization></affiliation></correspondence>` + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks())
	if !ok || len(parsed.Corresponding) != 0 || roles[2].Corresponding {
		t.Fatalf("affiliation-only correspondence must not assign a person: ok=%v roles=%+v", ok, roles)
	}
}

func TestXMLRolesUnmatchedCorrespondentNeedsReview(t *testing.T) {
	body := roleXMLPrefix + `<correspondence><person><given-name>Unknown</given-name><surname>Person</surname></person></correspondence>` + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resolveXMLRoles(parsed, sampleRoleLinks()); ok {
		t.Fatal("unmatched correspondence must not produce completed roles")
	}
}

func TestXMLRolesCreatorMismatchNeedsReview(t *testing.T) {
	body := roleXMLPrefix + roleXMLSuffix
	parsed, err := parseAuthorRoleXML([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	parsed.CreatorID = "22"
	if _, ok := resolveXMLRoles(parsed, sampleRoleLinks()); ok {
		t.Fatal("first author mismatch must not produce completed roles")
	}
}

func TestXMLRolesOnlyUsesUniqueNameFallbackWithoutAuthorID(t *testing.T) {
	parsed := parsedAuthorRoles{Authors: []xmlRolePerson{{Seq: 1, Given: "First", Surname: "Writer"}}}
	roles, ok := resolveXMLRoles(parsed, sampleRoleLinks()[:1])
	if !ok || !roles[1].First {
		t.Fatalf("unique name fallback failed: ok=%v roles=%+v", ok, roles)
	}
	links := []roleLink{{LinkID: 1, GivenName: "First", Surname: "Writer"}, {LinkID: 2, GivenName: "First", Surname: "Writer"}}
	if _, ok := uniqueLocalNameMatch(parsed.Authors[0], links); ok {
		t.Fatal("ambiguous name fallback must fail")
	}
}

func TestXMLRolesAuthorIDChangedButUniqueExactName(t *testing.T) {
	parsed := parsedAuthorRoles{Authors: []xmlRolePerson{{AuthorID: "new-id", Seq: 1, Given: "Chee-Hung", Surname: "Henry Chu"}}}
	links := []roleLink{{LinkID: 1, AuthorID: "old-id", GivenName: "Chee Hung", Surname: "Henry Chu"}}
	roles, ok := resolveXMLRoles(parsed, links)
	if !ok || !roles[1].First {
		t.Fatalf("expected unique exact name to bridge changed Scopus ID: ok=%v roles=%+v", ok, roles)
	}
	links = append(links, roleLink{LinkID: 2, AuthorID: "another-id", GivenName: "Chee Hung", Surname: "Henry Chu"})
	if _, ok := resolveXMLRoles(parsed, links); ok {
		t.Fatal("ambiguous duplicate name must remain needs_review")
	}
}

func TestScopusAuthorRosterDetectsReordering(t *testing.T) {
	a := &scopusEntry{Author: scopusAuthors{{AuthID: "11"}, {AuthID: "22"}}}
	b := &scopusEntry{Author: scopusAuthors{{AuthID: "22"}, {AuthID: "11"}}}
	if scopusAuthorRoster(a) == scopusAuthorRoster(b) {
		t.Fatal("reordered author list must invalidate stored roles")
	}
}

func TestVerifiedStaleRoleLinksRequiresSearchAndXMLAgreement(t *testing.T) {
	parsed := parsedAuthorRoles{Authors: []xmlRolePerson{{AuthorID: "11", Seq: 1}, {AuthorID: "22", Seq: 2}}}
	search := &scopusEntry{Author: scopusAuthors{{AuthID: "11"}, {AuthID: "22"}}}
	links := []roleLink{{LinkID: 1, AuthorID: "11"}, {LinkID: 2, AuthorID: "22"}, {LinkID: 3, AuthorID: "obsolete"}}
	current, stale, ok := verifiedStaleRoleLinks(parsed, search, links)
	if !ok || len(current) != 2 || len(stale) != 1 || stale[0] != 3 {
		t.Fatalf("expected one corroborated stale link, got current=%v stale=%v ok=%v", current, stale, ok)
	}
	search.Author[1].AuthID = "different"
	if _, _, ok := verifiedStaleRoleLinks(parsed, search, links); ok {
		t.Fatal("Search/XML disagreement must not remove any link")
	}
}
