package golang

/*
  A trace file is a zip containing critical profile information. To
  have more control over profile data uploaded use the "profile" VQL
  function, this function is designed to be a set and forget
  function.
*/

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"runtime/pprof"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"www.velocidex.com/golang/velociraptor/actions"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type TraceFunction struct{}

func (self *TraceFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "trace", args)()

	buf := new(bytes.Buffer)
	lf := []byte("\n")
	zipfile := zip.NewWriter(buf)

	// Cap the size of these dumps because the important info is near
	// the top.
	err := storeFile(zipfile, "allocs", 200000)
	if err != nil {
		scope.Log("trace: %v", err)
	}

	err = storeFile(zipfile, "goroutine", 200000)
	if err != nil {
		scope.Log("trace: %v", err)
	}

	fd, err := zipfile.Create("logs.txt")
	if err == nil {
		for _, line := range logging.GetMemoryLogs() {
			_, err = fd.Write([]byte(line))
			if err != nil {
				scope.Log("trace: %v", err)
			}
		}
	}

	// Show all the queries that recently ran
	fd, err = zipfile.Create("queries.jsonl")
	if err == nil {
		for _, line := range actions.QueryLog.Get() {
			serialized, err := json.Marshal(line)
			if err != nil {
				continue
			}

			_, _ = fd.Write(serialized)
			_, _ = fd.Write(lf)
		}
	}

	// Dump all binary metrics
	metrics, err := getMetrics()
	if err == nil {
		fd, err := zipfile.Create("metrics.jsonl")
		if err == nil {
			for _, metric := range metrics {
				metric.Delete("help")
				serialized, err := json.Marshal(metric)
				if err != nil {
					continue
				}
				_, _ = fd.Write(serialized)
				_, _ = fd.Write(lf)
			}
		}
	}

	zipfile.Close()

	subscope := scope.Copy()
	subscope.AppendVars(ordereddict.NewDict().
		Set("ZipFile", buf.Bytes()))

	// Allow the uploader to be overriden.
	upload_func, ok := scope.GetFunction("upload")
	if !ok {
		return &vfilter.Null{}
	}

	return upload_func.Call(
		ctx, subscope, ordereddict.NewDict().
			Set("accessor", "scope").
			Set("file", "ZipFile").
			Set("name", fmt.Sprintf("Trace%d.zip",
				utils.GetTime().Now().Unix())))
}

func getMetrics() ([]*ordereddict.Dict, error) {
	result := []*ordereddict.Dict{}
	gathering, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}

	for _, metric := range gathering {
		for _, m := range metric.Metric {
			item := ordereddict.NewDict().
				Set("name", *metric.Name).
				Set("help", *metric.Help)

			if m.Gauge != nil {
				item.Set("value", int64(*m.Gauge.Value))
				if len(m.Label) > 0 {
					labels := ordereddict.NewDict()
					for _, l := range m.Label {
						if l.Name != nil && l.Value != nil {
							labels.Set(*l.Name, l.Value)
						}
					}
					item.Set("label", labels)
				}

			} else if m.Counter != nil {
				item.Set("value", int64(*m.Counter.Value))
				if len(m.Label) > 0 {
					labels := ordereddict.NewDict()
					for _, l := range m.Label {
						if l.Name != nil && l.Value != nil {
							labels.Set(*l.Name, l.Value)
						}
					}
					item.Set("label", labels)
				}

			} else if m.Histogram != nil {
				// Histograms are buckets so we send a dict.
				result := ordereddict.NewDict()
				item.Set("value", result)

				label := "_"
				for _, l := range m.Label {
					label += *l.Value + "_"
				}

				for idx, b := range m.Histogram.Bucket {
					name := fmt.Sprintf("%v%v_%0.2f", *metric.Name,
						label, *b.UpperBound)
					if idx == len(m.Histogram.Bucket)-1 {
						name = fmt.Sprintf("%v%v_inf", *metric.Name,
							label)
					}
					result.Set(name, int64(*b.CumulativeCount))
				}

			} else if m.Summary != nil {
				result := ordereddict.NewDict().
					Set(fmt.Sprintf("%v_sample_count", *metric.Name),
						m.Summary.SampleCount)
				item.Set("value", result)

				for _, b := range m.Summary.Quantile {
					name := fmt.Sprintf("%v_%v", *metric.Name, *b.Quantile)
					result.Set(name, int64(*b.Value))
				}

			} else if m.Untyped != nil {
				item.Set("value", int64(*m.Untyped.Value))

			} else {
				// Unknown type just send the raw metric
				item.Set("value", m)
			}

			result = append(result, item)
		}
	}

	return result, nil
}

func storeFile(zipfile *zip.Writer, name string, max_len int) error {
	p := pprof.Lookup(name)
	if p == nil {
		return fmt.Errorf("profile type %s not known", name)
	}

	fd, err := zipfile.Create(name + ".txt")
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)

	err = p.WriteTo(buf, 1)
	if err != nil {
		return err
	}

	// Cap the size of the buffer at a reasonable size - we normally
	// only care about the top counts at the top of the file anyway.
	buf_bytes := buf.Bytes()
	if max_len > len(buf_bytes) {
		max_len = len(buf_bytes)
	}
	_, err = fd.Write(buf_bytes[:max_len])
	return err
}

func (self TraceFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "trace",
		Doc:     "Upload a trace file.",
		Version: 1,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TraceFunction{})
}
