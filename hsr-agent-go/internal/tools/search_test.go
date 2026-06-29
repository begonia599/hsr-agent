package tools

import "testing"

func TestSemanticKeywordTermsNormalizeDomainTerms(t *testing.T) {
	terms := semanticKeywordTerms("同协开拓者 击破套装 防御无视")
	for _, want := range []string{"同谐", "开拓者", "击破", "套装", "无视防御"} {
		if !containsSearchTerm(terms, want) {
			t.Fatalf("semanticKeywordTerms missing %q in %#v", want, terms)
		}
	}
}

func TestMergeSemanticCandidatesCombinesRecallSources(t *testing.T) {
	rows := mergeSemanticCandidates(
		[]SemanticMatch{{
			Kind:         "character",
			ID:           1310,
			NameZH:       "流萤",
			Score:        0.61,
			RecallScore:  0.61,
			RecallSource: "embedding",
		}},
		[]SemanticMatch{{
			Kind:          "character",
			ID:            1310,
			NameZH:        "流萤",
			Score:         0.92,
			RecallScore:   0.92,
			RecallSource:  "keyword",
			CandidateText: "流萤\n击破\n超击破",
			URL:           "/characters/1310",
			Markdown:      "[流萤](/characters/1310)",
		}},
	)
	if len(rows) != 1 {
		t.Fatalf("len = %d, want 1", len(rows))
	}
	if rows[0].RecallSource != "embedding+keyword" {
		t.Fatalf("RecallSource = %q, want embedding+keyword", rows[0].RecallSource)
	}
	if rows[0].RecallScore != 0.92 {
		t.Fatalf("RecallScore = %v, want 0.92", rows[0].RecallScore)
	}
	if rows[0].CandidateText == "" {
		t.Fatal("CandidateText should be filled from supplemental candidate")
	}
	if rows[0].URL != "/characters/1310" || rows[0].Markdown == "" {
		t.Fatalf("entity link was not preserved: url=%q markdown=%q", rows[0].URL, rows[0].Markdown)
	}
}

func containsSearchTerm(terms []string, want string) bool {
	want = normalizeSearchToken(want)
	for _, term := range terms {
		if normalizeSearchToken(term) == want {
			return true
		}
	}
	return false
}
