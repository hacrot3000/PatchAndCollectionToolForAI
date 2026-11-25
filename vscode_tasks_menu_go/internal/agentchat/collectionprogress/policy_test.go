package collectionprogress

import "testing"

func TestReadAdvisoriesNeverCapUsefulReads(t *testing.T) {
	for _, intent := range []Intent{Implementation, Mixed, ReadOnly, UnknownIntent} {
		t.Run(string(intent), func(t *testing.T) {
			var state State
			advisories := map[uint64]Reason{}
			for i := uint64(1); i <= 100; i++ {
				var decision Decision
				state, decision = Apply(Defaults(), intent, state, Observation{Collected: true, Novelty: VerifiedNew})
				if decision.Kind == Advise {
					advisories[i] = decision.Reason
				} else if decision.Kind != Continue {
					t.Fatalf("read %d denied: %+v", i, decision)
				}
			}
			if intent == Implementation || intent == Mixed {
				if len(advisories) != 2 || advisories[5] != AssessReadiness || advisories[10] != BoundNextScope {
					t.Fatalf("wrong thresholds: %v", advisories)
				}
			} else if len(advisories) != 0 {
				t.Fatalf("read-only/unknown got implementation advice: %v", advisories)
			}
			if state.ReadsSinceProgress != 100 {
				t.Fatalf("lost observations: %+v", state)
			}
		})
	}
}

func TestVerifiedDuplicateCooldownAndUnknown(t *testing.T) {
	var state State
	observe := func(n Novelty) Decision {
		var d Decision
		state, d = Apply(Defaults(), ReadOnly, state, Observation{Collected: true, Novelty: n})
		return d
	}
	for _, n := range []Novelty{VerifiedDuplicate, VerifiedDuplicate, UnknownNovelty, VerifiedDuplicate, VerifiedDuplicate, ObservationError, VerifiedDuplicate, VerifiedDuplicate} {
		if d := observe(n); d.Kind != Continue {
			t.Fatalf("unverified streak: %+v", d)
		}
	}
	if d := observe(VerifiedDuplicate); d.Reason != RepeatedEvidence {
		t.Fatalf("missing duplicate warning: %+v", d)
	}
	for i := 0; i < 4; i++ {
		if d := observe(VerifiedDuplicate); d.Kind != Continue {
			t.Fatalf("cooldown spam: %+v", d)
		}
	}
	if d := observe(VerifiedDuplicate); d.Reason != RepeatedEvidence {
		t.Fatalf("cooldown never expires: %+v", d)
	}
	if d := observe(VerifiedNew); d.Kind != Continue || state.DuplicateStreak != 0 {
		t.Fatalf("novel evidence must break streak: %+v %+v", d, state)
	}
	if state.ReadsSinceProgress != 15 {
		t.Fatalf("new evidence is not objective progress: %+v", state)
	}
}

func TestOnlyMeaningfulProgressResetsObjective(t *testing.T) {
	state := State{ReadsSinceProgress: 12, DuplicateStreak: 3, FirstAdvised: true, SecondAdvised: true}
	unchanged, d := Apply(Defaults(), Implementation, state, Observation{})
	if unchanged != state || d.Kind != Continue {
		t.Fatal("non-observation reset objective")
	}
	reset, d := Apply(Defaults(), Implementation, state, Observation{MeaningfulProgress: true})
	if reset != (State{}) || d.Kind != Continue {
		t.Fatal("verified progress failed to reset objective")
	}
	newState, d := Apply(Policy{}, Implementation, State{ReadsSinceProgress: 4}, Observation{Collected: true, Novelty: UnknownNovelty})
	if newState.ReadsSinceProgress != 5 || d.Reason != AssessReadiness {
		t.Fatal("invalid config failed safe defaults")
	}
}
