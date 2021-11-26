package vtesting

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func GetMetrics(t *testing.T, name_regex string) *ordereddict.Dict {
	return GetMetricsDifference(t, name_regex, nil)
}

func GetMetricsDifference(t *testing.T,
	name_regex string, snapshot *ordereddict.Dict) *ordereddict.Dict {
	if snapshot == nil {
		snapshot = ordereddict.NewDict()
	}

	regex, err := regexp.Compile(name_regex)
	assert.NoError(t, err)

	result := ordereddict.NewDict()

	gathering, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)

	for _, metric := range gathering {
		for _, m := range metric.Metric {
			if m.Gauge != nil {
				value := int64(*m.Gauge.Value)
				old_value, pres := snapshot.GetInt64(*metric.Name)
				if pres {
					value -= old_value
				}
				if regex.MatchString(*metric.Name) {
					result.Set(*metric.Name, value)
				}

			} else if m.Counter != nil {
				value := int64(*m.Counter.Value)
				old_value, pres := snapshot.GetInt64(*metric.Name)
				if pres {
					value -= old_value
				}
				if regex.MatchString(*metric.Name) {
					result.Set(*metric.Name, value)
				}
			} else if m.Histogram != nil {

				label := ""
				for _, l := range m.Label {
					label += "_" + *l.Value
				}

				for idx, b := range m.Histogram.Bucket {
					name := fmt.Sprintf("%v_%v_%0.2f", *metric.Name,
						label, *b.UpperBound)
					if idx == len(m.Histogram.Bucket)-1 {
						name = fmt.Sprintf("%v_%v_inf", *metric.Name,
							label)
					}
					value := int64(*b.CumulativeCount)
					old_value, pres := snapshot.GetInt64(name)
					if pres {
						value -= old_value
					}
					if regex.MatchString(name) {
						result.Set(name, value)
					}
				}
			}
		}
	}
	return result
}
