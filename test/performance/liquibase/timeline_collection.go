package benchmark

import "strconv"

// costTimelineAux is a closed client lane on the common relative clock. It
// contains no URL, request payload, process ID, or absolute host reading.
type costTimelineAux struct {
	Kind           string
	StartNS, EndNS int64
}

// costTimelineCapture observes existing calls, not replacement HTTP/sleep
// implementations. A failed read poisons the whole pass (TL01/TL08).
type costTimelineCapture struct {
	read      func() (int64, error)
	intervals []costTimelineAux
	err       error
}

func validateTimelineSelection(cost, timeline bool) error {
	if timeline && !cost {
		return errCostStream
	}
	return nil
}

// costTimelineEnvironment strips inherited selectors even in unselected
// routes. The absolute reference is private transport, never report data.
func costTimelineEnvironment(input []string, enabled bool, reference int64) []string {
	result := costEngineEnvironment(input, enabled)
	if enabled {
		result = append(result, "SQLRS_PREPARE_TIMELINE=1", "SQLRS_PREPARE_TIMELINE_REFERENCE="+strconv.FormatInt(reference, 10))
	}
	return result
}

func (capture *costTimelineCapture) begin(kind string) func() {
	if capture == nil {
		return func() {}
	}
	if capture.err != nil {
		return func() {}
	}
	if !costTimelineClientKind(kind) || len(capture.intervals) >= 8192 {
		capture.err = errCostStream
		return func() {}
	}
	start, err := capture.read()
	if err != nil || start < 0 || uint64(start) > maxCostTimelineNumber {
		capture.err = errCostStream
		return func() {}
	}
	return func() {
		end, err := capture.read()
		if err != nil || end < start || uint64(end) > maxCostTimelineNumber {
			capture.err = errCostStream
			return
		}
		capture.intervals = append(capture.intervals, costTimelineAux{kind, start, end})
	}
}

func costTimelineClientKind(kind string) bool {
	return kind == "client.post" || kind == "client.status" || kind == "client.poll"
}

// qualifyTimelinePass validates identity, both execution participants, finite
// shared-clock placement and original-timer reconciliation. Arithmetic alone
// cannot turn an unrelated request or partial capture into workload evidence.
func qualifyTimelinePass(root costRoot, ordinal uint64, timer costTimelineTimer, intervals []costTimelineAux, changes int) error {
	if _, err := qualifyTimelineTimer(timer); err != nil {
		return err
	}
	if root.Root != ordinal || !root.Complete || !root.Ended || root.Clock == nil ||
		root.Clock.Domain != "linux-monotonic-relative-v1" || root.Clock.OriginNS < timer.Window.StartNS ||
		root.Clock.OriginNS >= timer.Window.EndNS || root.ElapsedNS <= 0 ||
		uint64(root.Clock.OriginNS) > maxCostTimelineNumber || uint64(root.ElapsedNS) > maxCostTimelineNumber-uint64(root.Clock.OriginNS) ||
		root.ObserverWindow == nil || root.ObserverWindow.StartNS < root.Clock.OriginNS+root.ElapsedNS ||
		root.ObserverWindow.EndNS < root.ObserverWindow.StartNS || uint64(root.ObserverWindow.EndNS) > maxCostTimelineNumber ||
		changes <= 0 || len(root.Spans) < 3 || len(root.Spans) != root.Declared {
		return errCostStream
	}
	request := root.Spans[0]
	if request.ID != 1 || request.Parent != 0 || request.Operation != "prepare.request" || request.Phase != "worker" ||
		request.Step != 0 || request.Outcome != "succeeded" || request.StartNS < 0 || request.StartNS > request.EndNS || request.EndNS != root.ElapsedNS {
		return errCostStream
	}
	admission, worker := 0, 0
	for i, span := range root.Spans[1:] {
		if span.ID != i+2 || span.Parent <= 0 || span.Parent >= span.ID || span.Step < 0 || span.Step > changes ||
			span.StartNS < 0 || span.EndNS < span.StartNS || span.EndNS > root.ElapsedNS {
			return errCostStream
		}
		parent := root.Spans[span.Parent-1]
		if span.StartNS < parent.StartNS || span.EndNS > parent.EndNS {
			return errCostStream
		}
		if span.Operation == "prepare.admission" || span.Operation == "prepare.worker" {
			if span.Parent != 1 || span.Step != 0 || span.Outcome != "succeeded" {
				return errCostStream
			}
			if span.Operation == "prepare.admission" {
				if span.Phase != "admission" {
					return errCostStream
				}
				admission++
			} else {
				if span.Phase != "worker" {
					return errCostStream
				}
				worker++
			}
		}
	}
	if admission != 1 || worker != 1 || len(intervals) < 2 || len(intervals) > 8192 {
		return errCostStream
	}
	post, status, previous := 0, 0, timer.Window.StartNS
	for _, interval := range intervals {
		if !costTimelineClientKind(interval.Kind) || interval.StartNS < previous || interval.EndNS < interval.StartNS || interval.EndNS > timer.Window.EndNS {
			return errCostStream
		}
		if interval.Kind == "client.post" {
			post++
		}
		if interval.Kind == "client.status" {
			status++
		}
		previous = interval.EndNS
	}
	if post != 1 || status == 0 || intervals[0].Kind != "client.post" || intervals[len(intervals)-1].Kind != "client.status" {
		return errCostStream
	}
	return qualifyDetails(root)
}
