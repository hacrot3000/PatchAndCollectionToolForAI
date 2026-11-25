package collectionprogress

import "testing"

func TestRecoveryAfterVerifiedDuplicateAdvisoryAndCooldown(t *testing.T) {
	s := counterFixture(t)
	var recovery RecoveryState
	gate := RecoveryGate{SafeEgress: true, CanonicalOutputCommitted: true, OwnerAllowsContinuation: true}
	for i := uint64(1); i <= 13; i++ {
		e := counterEvent(i, CollectionObserved, VerifiedDuplicate, true)
		next, _, applied, err := ApplyCounterEvent(Defaults(), s, e, true)
		if err != nil || !applied { t.Fatal(err) }
		s = next
		var d RecoveryDecision
		recovery, d = AssessRecovery(Defaults(), s, e, recovery, gate)
		switch i {
		case 8:
			if d.Kind != Replan || d.NextAction != NarrowMissingScope || recovery.Attempts != 1 { t.Fatalf("no bounded replan: %+v %+v", recovery, d) }
			_, repeat := AssessRecovery(Defaults(), s, e, recovery, gate)
			if repeat.Kind != Checkpoint || repeat.Reason != RecoveryAlreadyRecorded { t.Fatal("replayed recovery intent") }
		case 13:
			if d.Kind != Checkpoint || d.Reason != RecoveryBudgetExhausted { t.Fatalf("unbounded recovery: %+v", d) }
		default:
			if d.Kind != Continue { t.Fatalf("premature checkpoint on read %d: %+v", i, d) }
		}
	}
	e := counterEvent(14, CollectionObserved, VerifiedNew, true)
	s, _, _, _ = ApplyCounterEvent(Defaults(), s, e, true)
	next, d := AssessRecovery(Defaults(), s, e, recovery, gate)
	if d.Kind != Continue || next.Attempts != 1 { t.Fatal("novel read blocked or generation recovery budget reset") }
}

func TestRecoveryGatesAndUnknownNeverAuthorizeNewEgress(t *testing.T) {
	s := counterFixture(t)
	e := counterEvent(1, CollectionObserved, UnknownNovelty, true)
	s, _, _, _ = ApplyCounterEvent(Defaults(), s, e, true)
	for _, tc := range []struct{gate RecoveryGate; kind DecisionKind; reason Reason}{
		{RecoveryGate{Stopped:true}, Checkpoint, RecoveryStopped},
		{RecoveryGate{CanonicalOutputCommitted:true}, Checkpoint, RecoveryUnknownEgress},
		{RecoveryGate{SafeEgress:true}, Checkpoint, RecoveryPendingCall},
		{RecoveryGate{SafeEgress:true, CanonicalOutputCommitted:true, OwnerAllowsContinuation:true}, Continue, ""},
	} {
		got, d := AssessRecovery(Defaults(), s, e, RecoveryState{}, tc.gate)
		if got.Attempts != 0 || d.Kind != tc.kind || d.Reason != tc.reason { t.Fatalf("unsafe recovery: %+v %+v", got,d) }
	}
	for i:=uint64(2);i<=9;i++{
		e=counterEvent(i,CollectionObserved,VerifiedDuplicate,true)
		s,_,_,_=ApplyCounterEvent(Defaults(),s,e,true)
	}
	got,d:=AssessRecovery(Defaults(),s,e,RecoveryState{},RecoveryGate{SafeEgress:true,CanonicalOutputCommitted:true})
	if got.Attempts!=0||d.Reason!=RecoveryNotAuthorized||d.Kind!=Checkpoint{t.Fatalf("consent bypass: %+v %+v",got,d)}
}

func TestRecoveryPolicyBoundsAreNotReadLimits(t *testing.T) {
	p:=Defaults()
	p.DuplicateCooldown=^uint64(0)
	if p.Valid(){t.Fatal("overflowing policy accepted")}
	s:=counterFixture(t)
	var recovery RecoveryState
	for i:=uint64(1);i<=200;i++{
		e:=counterEvent(i,CollectionObserved,VerifiedNew,true)
		s,_,_,_=ApplyCounterEvent(p,s,e,true)
		var d RecoveryDecision
		recovery,d=AssessRecovery(p,s,e,recovery,RecoveryGate{SafeEgress:true,CanonicalOutputCommitted:true,OwnerAllowsContinuation:true})
		if d.Kind!=Continue{t.Fatalf("unique read %d capped: %+v",i,d)}
	}
	if s.NativeReads!=200||recovery.Attempts!=0{t.Fatal("unexpected recovery on new evidence")}
}
