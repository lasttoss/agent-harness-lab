package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	numberRe = regexp.MustCompile(`\d+(?:[.,_]\d+)*\s*%?`)
	// "unknown" is deliberately not in this list: it is a legitimate part of a
	// correct answer ("an unmapped file is an unknown, not a no"). Only phrases that
	// mean the model is declining to answer belong here.
	unsureRe = regexp.MustCompile(`(?i)\b(unsure|not sure|i don't know|cannot determine|cannot tell|no information)\b`)
)

func normalise(s string) string {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToLower(s)
}

// Verdict is the check the loop applies to its own model's answer before spending
// another call. It is deliberately cheap and deterministic: a regex written before
// the run, applied to the reply, against the evidence the task supplied.
type Verdict struct {
	OK     bool
	Reason string
}

func Judge(evidence, reply, expect string) Verdict {
	return judgeWith(evidence, reply, expect, false)
}

// judgeWith adds one switch the gate needs in practice: some answers are computed
// from the evidence rather than quoted from it. For those, an absent number is not
// proof of invention — the expectation regex becomes the check instead.
func judgeWith(evidence, reply, expect string, derived bool) Verdict {
	if len(strings.TrimSpace(reply)) < 10 {
		return Verdict{false, "empty_reply"}
	}
	if unsureRe.MatchString(reply) {
		return Verdict{false, "model_unsure"}
	}
	ev := normalise(evidence)
	for _, n := range numberRe.FindAllString(reply, -1) {
		core := strings.TrimSuffix(normalise(n), "%")
		if len(core) == 1 { // a bare digit carries no claim
			continue
		}
		if !strings.Contains(ev, core) {
			if derived && expect != "" {
				if re, err := regexp.Compile(expect); err == nil && re.MatchString(reply) {
					continue
				}
			}
			return Verdict{false, "unsupported_number: " + strings.TrimSpace(n)}
		}
	}
	if expect != "" {
		re, err := regexp.Compile(expect)
		if err != nil {
			return Verdict{false, "bad_expectation: " + err.Error()}
		}
		if !re.MatchString(reply) {
			return Verdict{false, fmt.Sprintf("answer_mismatch: expected /%s/", expect)}
		}
	}
	return Verdict{true, "ok"}
}
