package deployer

import (
	"context"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/spectrocloud-labs/herd"
)

// convertEntry registers the raw-disk stub plus the GCE/VHD/MAAS convert steps
// for the given disk config and returns the graph entry for opName.
func convertEntry(t *testing.T, disk schema.Disk, opName string) herd.GraphEntry {
	t.Helper()
	d := NewDeployer(schema.Config{Disk: disk}, schema.ReleaseArtifact{})
	if err := d.Add(constants.OpGenEFIRawDisk, herd.WithCallback(func(context.Context) error { return nil })); err != nil {
		t.Fatalf("add raw-disk stub: %v", err)
	}
	for _, add := range []func() error{d.StepConvertMAAS, d.StepConvertGCE, d.StepConvertVHD} {
		if err := add(); err != nil {
			t.Fatalf("register convert step: %v", err)
		}
	}
	for _, layer := range d.Analyze() {
		for _, op := range layer {
			if op.Name == opName {
				return op
			}
		}
	}
	t.Fatalf("step %q not found in graph", opName)
	return herd.GraphEntry{}
}

func dependsOn(deps []string, want string) bool {
	for _, d := range deps {
		if d == want {
			return true
		}
	}
	return false
}

// TestConvertGCEDependsOnMAASWhenBoth guards the read-vs-truncate race: MAAS
// reads the raw while GCE truncates it in place, so when both are requested GCE
// must run after MAAS.
func TestConvertGCEDependsOnMAASWhenBoth(t *testing.T) {
	gce := convertEntry(t, schema.Disk{GCE: true, MAAS: true}, constants.OpConvertGCE)
	if !dependsOn(gce.Dependencies, constants.OpConvertMAAS) {
		t.Fatalf("convert-gce must depend on convert-maas when both requested; deps=%v", gce.Dependencies)
	}
}

// TestConvertVHDDependsOnMAASWhenBoth guards the read-vs-rename race: MAAS reads
// the raw while VHD renames it away, so when both are requested VHD must run
// after MAAS.
func TestConvertVHDDependsOnMAASWhenBoth(t *testing.T) {
	vhd := convertEntry(t, schema.Disk{VHD: true, MAAS: true}, constants.OpConvertVHD)
	if !dependsOn(vhd.Dependencies, constants.OpConvertMAAS) {
		t.Fatalf("convert-vhd must depend on convert-maas when both requested; deps=%v", vhd.Dependencies)
	}
}

// TestConvertDoesNotDependOnMAASWhenMAASDisabled guards the conditional: a
// GCE/VHD build without MAAS must not depend on the disabled MAAS step (which
// would never run, wedging the build).
func TestConvertDoesNotDependOnMAASWhenMAASDisabled(t *testing.T) {
	gce := convertEntry(t, schema.Disk{GCE: true}, constants.OpConvertGCE)
	if dependsOn(gce.Dependencies, constants.OpConvertMAAS) {
		t.Fatalf("GCE-only build must not depend on convert-maas; deps=%v", gce.Dependencies)
	}
	vhd := convertEntry(t, schema.Disk{VHD: true}, constants.OpConvertVHD)
	if dependsOn(vhd.Dependencies, constants.OpConvertMAAS) {
		t.Fatalf("VHD-only build must not depend on convert-maas; deps=%v", vhd.Dependencies)
	}
}

// TestConvertMAASOnlyNotWedged confirms a MAAS-only build registers and is not
// ignored (it has no downstream convert dependents).
func TestConvertMAASOnlyNotWedged(t *testing.T) {
	maas := convertEntry(t, schema.Disk{MAAS: true}, constants.OpConvertMAAS)
	if maas.Ignored {
		t.Fatal("MAAS-only build must not ignore the convert-maas step")
	}
}
