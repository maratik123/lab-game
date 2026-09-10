package health

import (
	"bytes"
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// gatherFrom registers c on a fresh, isolated registry and gathers it —
// the same isolation testutil.CollectAndCompare and testutil.CollectAndCount
// give a single collector, exposed here for assertions those two cannot
// make directly: an exact observed label-value set, or a byte-for-byte
// scrape comparison across two different collectors.
func gatherFrom(t *testing.T, c prometheus.Collector) ([]*dto.MetricFamily, error) {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		return nil, err
	}
	return reg.Gather()
}

// observedLabelValues returns the set of values labelName takes across
// every metric in mfs.
func observedLabelValues(mfs []*dto.MetricFamily, labelName string) map[string]bool {
	values := map[string]bool{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == labelName {
					values[lp.GetValue()] = true
				}
			}
		}
	}
	return values
}

// gatherText renders mfs as Prometheus text exposition, sorted by family
// name so the byte comparison does not depend on gather order.
func gatherText(t *testing.T, mfs []*dto.MetricFamily) string {
	t.Helper()
	sorted := make([]*dto.MetricFamily, len(mfs))
	copy(sorted, mfs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].GetName() < sorted[j].GetName() })
	var buf bytes.Buffer
	for _, mf := range sorted {
		if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
			t.Fatalf("MetricFamilyToText: %v", err)
		}
	}
	return buf.String()
}
