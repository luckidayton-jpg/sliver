package generate

import (
	"testing"

	"github.com/bishopfox/sliver/protobuf/clientpb"
)

func TestNameOfOutputFormatGoArchive(t *testing.T) {
	if got := nameOfOutputFormat(clientpb.OutputFormat_GO_ARCHIVE); got != "Go Archive" {
		t.Fatalf("nameOfOutputFormat(GO_ARCHIVE) = %q, want %q", got, "Go Archive")
	}
}

func TestNameOfOutputFormatThirdParty(t *testing.T) {
	if got := nameOfOutputFormat(clientpb.OutputFormat_THIRD_PARTY); got != "Third Party" {
		t.Fatalf("nameOfOutputFormat(THIRD_PARTY) = %q, want %q", got, "Third Party")
	}
}
