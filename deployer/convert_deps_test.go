package deployer

import (
	"context"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/spectrocloud-labs/herd"
)

// convertVHDEntry registers the GCE + VHD convert steps (on top of a stub
// raw-disk step so their shared dependency resolves) for the given disk config
// and returns the resolved convert-vhd graph entry.
func convertVHDEntry(t *testing.T, disk schema.Disk) herd.GraphEntry {
	t.Helper()
	d := NewDeployer(schema.Config{Disk: disk}, schema.ReleaseArtifact{})
	// Both converts depend on the raw disk; stub it so the graph resolves
	// without registering the whole build chain.
	if err := d.Add(constants.OpGenEFIRawDisk, herd.WithCallback(func(context.Context) error { return nil })); err != nil {
		t.Fatalf("add raw-disk stub: %v", err)
	}
	if err := d.StepConvertGCE(); err != nil {
		t.Fatalf("StepConvertGCE: %v", err)
	}
	if err := d.StepConvertVHD(); err != nil {
		t.Fatalf("StepConvertVHD: %v", err)
	}
	for _, layer := range d.Analyze() {
		for _, op := range layer {
			if op.Name == constants.OpConvertVHD {
				return op
			}
		}
	}
	t.Fatal("convert-vhd step not found in graph")
	return herd.GraphEntry{}
}

func hasDep(deps []string, want string) bool {
	for _, d := range deps {
		if d == want {
			return true
		}
	}
	return false
}

// TestConvertVHDDependsOnGCEWhenBothRequested is the regression guard for the
// kairos #4117 item: GCE and VHD both consume the single `kairos-*.raw` in place
// (GCE truncates it, VHD renames it away), so running them in parallel corrupts
// the raw or makes VHD's rename remove the file GCE still needs. When both are
// requested, VHD must be ordered after GCE.
func TestConvertVHDDependsOnGCEWhenBothRequested(t *testing.T) {
	vhd := convertVHDEntry(t, schema.Disk{GCE: true, VHD: true})
	if !hasDep(vhd.Dependencies, constants.OpConvertGCE) {
		t.Fatalf("convert-vhd must depend on convert-gce when both are requested; deps=%v", vhd.Dependencies)
	}
}

// TestConvertVHDDoesNotDependOnGCEWhenVHDOnly guards the other half: the GCE
// dependency is conditional, so a VHD-only build must not depend on the disabled
// GCE step (which would otherwise never run, wedging the build).
func TestConvertVHDDoesNotDependOnGCEWhenVHDOnly(t *testing.T) {
	vhd := convertVHDEntry(t, schema.Disk{VHD: true})
	if hasDep(vhd.Dependencies, constants.OpConvertGCE) {
		t.Fatalf("VHD-only build must not depend on convert-gce; deps=%v", vhd.Dependencies)
	}
	if vhd.Ignored {
		t.Fatal("VHD-only build must not ignore the convert-vhd step")
	}
}
