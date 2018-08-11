package vql

import (
	"context"
	"github.com/Velocidex/ahocorasick"
	"io"
	"www.velocidex.com/golang/velociraptor/glob"
	"www.velocidex.com/golang/vfilter"
)

type GrepFunction struct{}

// The Grep VQL function searches for a literal or regex match inside the file
func (self *GrepFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {

	path, pres := vfilter.ExtractString("path", args)
	if !pres {
		scope.Log("Arg path not specified")
		return false
	}

	keywords_str, pres := vfilter.ExtractStringArray(scope, "keywords", args)
	if !pres {
		scope.Log("Arg keywords not specified")
		return false
	}

	var keywords [][]byte
	for _, item := range keywords_str {
		// TODO: Add extra encodings like UTF16
		keywords = append(keywords, []byte(item))
	}

	ah_matcher := ahocorasick.NewMatcher(keywords)
	offset := 0

	buf := make([]byte, 4*1024*1024) // 4Mb chunks
	fs := glob.OSFileSystemAccessor{}
	file, err := fs.Open(*path)
	if err != nil {
		return false
	}
	defer file.Close()

	hits := []*vfilter.Dict{}

	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			return false
		}

		for _, hit := range ah_matcher.Match(buf[:n]) {
			min_bound := offset + hit - 10
			if min_bound < 0 {
				min_bound = 0
			}

			max_bound := offset + hit + 10
			if max_bound > n {
				max_bound = n
			}

			hits = append(hits, vfilter.NewDict().
				Set("type", "GrepHit").
				Set("offset", offset+hit).
				Set("min_bound", min_bound).
				Set("max_bound", max_bound).
				Set("context", buf[min_bound:max_bound]))
		}

		offset += n
	}

	return hits
}

func (self GrepFunction) Name() string {
	return "grep"
}

func init() {
	exportedFunctions = append(exportedFunctions, &GrepFunction{})
}
