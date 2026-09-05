package nodes

import "testing"

func TestLogLevelPatchPreservesDisabledNode(t *testing.T) {
	f := newProbeServiceFixture(t, 512*1024*1024)
	before, err := f.service.SetEnabled(f.ctx, f.registration.NodeID, false)
	if err != nil {
		t.Fatal(err)
	}
	level := "debug"
	after, err := f.service.Update(f.ctx, before.ID, nil, nil, &level)
	if err != nil {
		t.Fatal(err)
	}
	if after.Enabled || after.Name != before.Name || after.LogLevel != level || after.DesiredConfigurationRevision != before.DesiredConfigurationRevision+1 {
		t.Fatalf("log level patch changed unrelated state: before=%+v after=%+v", before, after)
	}
}
