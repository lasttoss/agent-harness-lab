package main

import "testing"

const ev = "Tests run: 20, Failures: 0. LINE 107/120 = 89.2%. services=4 edges=7 impact_keys=12."

func TestJudge(t *testing.T) {
	cases := []struct {
		name, reply, expect string
		ok                  bool
		reason              string
	}{
		{"answer with evidence numbers", "Line coverage is 89.2% (107/120).", `89\.2`, true, "ok"},
		{"invented number", "Line coverage is 94.7%.", "", false, "unsupported_number: 94.7%"},
		{"bare digit is not a claim", "There are 3 services.", "", true, "ok"},
		{"empty", "ok", "", false, "empty_reply"},
		{"model admits uncertainty", "I cannot determine the coverage from this evidence.", "", false, "model_unsure"},
		{"right numbers, wrong answer", "The service count is 4.", `impact_keys=99`, false, "answer_mismatch: expected /impact_keys=99/"},
	}
	for _, c := range cases {
		got := Judge(ev, c.reply, c.expect)
		if got.OK != c.ok || got.Reason != c.reason {
			t.Errorf("%s: got ok=%v reason=%q, want ok=%v reason=%q", c.name, got.OK, got.Reason, c.ok, c.reason)
		}
	}
}

// A comma group in the reply must not be read as a different number than the
// evidence: "1,234" and "1234" are the same claim.
func TestJudgeCommaGroup(t *testing.T) {
	if v := Judge("peak bytes 1234567 with 42 classes", "Peak was 1,234,567 bytes across 42 classes.", ""); !v.OK {
		t.Fatalf("comma group rejected: %s", v.Reason)
	}
}

// A computed answer is rejected unless the task says the answer is derived, in
// which case the expectation regex is what decides.
func TestDerivedAnswer(t *testing.T) {
	if v := Judge("4 of 12 keys are tables", "Tables are 33.3% of the keys.", `(?i)33`); v.OK {
		t.Fatal("derived number should be rejected by the strict gate")
	}
	if v := judgeWith("4 of 12 keys are tables", "Tables are 33.3% of the keys.", `(?i)33`, true); !v.OK {
		t.Fatalf("derived answer rejected: %s", v.Reason)
	}
}
