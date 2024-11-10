package parsers

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"www.velocidex.com/golang/go-ntfs/parser"
	"www.velocidex.com/golang/velociraptor/accessors"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/protocols"
)

var (
	mftComponentsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "mft_components_count",
		Help: "Number of times we trace an MFT entry's parents.",
	})
)

type _MFTHighlightAssociative struct{}

func (self _MFTHighlightAssociative) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(*parser.MFTHighlight)
	if !a_ok {
		return false
	}

	_, b_ok := b.(string)
	if !b_ok {
		return false
	}

	return true
}

func (self _MFTHighlightAssociative) Associative(
	scope vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	hl, a_ok := a.(*parser.MFTHighlight)
	if !a_ok {
		return vfilter.Null{}, false
	}

	member, b_ok := b.(string)
	if !b_ok {
		return vfilter.Null{}, false
	}

	switch member {
	case "OSPath":
		mftComponentsCounter.Inc()
		return accessors.MustNewWindowsNTFSPath("").Append(hl.Components()...), true

		// This is for backwards compatibility with older VQL but we
		// do not advertize this field.
	case "FullPath":
		mftComponentsCounter.Inc()
		return strings.Join(hl.Components(), "\\"), true

	case "FileName":
		return hl.FileName(), true

	default:
		return protocols.DefaultAssociative{}.Associative(scope, a, b)
	}
}

func (self _MFTHighlightAssociative) GetMembers(
	scope vfilter.Scope, a vfilter.Any) []string {
	return []string{
		"EntryNumber",
		"OSPath",
		"SequenceNumber",
		"InUse",
		"ParentEntryNumber",
		"ParentSequenceNumber",
		"FileName",
		"FileNames",
		"Links",
		"FileNameTypes",
		"FileSize",
		"ReferenceCount",
		"IsDir",
		"HasADS",
		"SI_Lt_FN",
		"USecZeros",
		"Copied",
		"SIFlags",
		"Created0x10",
		"Created0x30",
		"LastModified0x10",
		"LastModified0x30",
		"LastRecordChange0x10",
		"LastRecordChange0x30",
		"LastAccess0x10",
		"LastAccess0x30",
		"LogFileSeqNum",
	}
}

func init() {
	vql_subsystem.RegisterProtocol(&_MFTHighlightAssociative{})
}
