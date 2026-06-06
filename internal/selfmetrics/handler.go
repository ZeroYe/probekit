package selfmetrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

type TargetCounter interface {
	Counts() map[string]int
}

func Handler(collectRuntime bool, tc TargetCounter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var targets map[string]int
		if tc != nil {
			targets = tc.Counts()
		}
		ms := Collect(collectRuntime, targets)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for _, m := range ms {
			writeMetric(w, m)
		}
	}
}

func writeMetric(w http.ResponseWriter, m SelfMetric) {
	fmt.Fprintf(w, "# HELP %s\n# TYPE %s %s\n", m.Name, m.Name, m.Type)
	w.Write([]byte(m.Name))
	if len(m.Labels) > 0 {
		keys := make([]string, 0, len(m.Labels))
		for k := range m.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.Write([]byte{'{'})
		for i, k := range keys {
			if i > 0 {
				w.Write([]byte{','})
			}
			w.Write([]byte(k))
			w.Write([]byte(`="`))
			w.Write([]byte(m.Labels[k]))
			w.Write([]byte{'"'})
		}
		w.Write([]byte{'}'})
	}
	w.Write([]byte{' '})
	w.Write([]byte(strconv.FormatFloat(m.Value, 'f', -1, 64)))
	w.Write([]byte{'\n'})
}
