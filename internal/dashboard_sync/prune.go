package sync

import "sync"

var pruneMu sync.Mutex
var pruneCandidates = map[string]map[string]int{} // dashboardID -> serviceName -> consecutive-miss-count

// PruneThreshold is the number of consecutive sync cycles a service must be missing
// before its panel is actually pruned. Default is conservative (3 cycles).
var PruneThreshold = 7

// incrPruneCandidate increments the miss counter for a dashboard/service and
// returns the new count.
func incrPruneCandidate(dashboardID, service string) int {
	pruneMu.Lock()
	defer pruneMu.Unlock()
	m, ok := pruneCandidates[dashboardID]
	if !ok {
		m = map[string]int{}
		pruneCandidates[dashboardID] = m
	}
	m[service] = m[service] + 1
	return m[service]
}

// resetPruneCandidate clears the miss counter for a dashboard/service.
func resetPruneCandidate(dashboardID, service string) {
	pruneMu.Lock()
	defer pruneMu.Unlock()
	if m, ok := pruneCandidates[dashboardID]; ok {
		delete(m, service)
	}
}

// shouldPrune reports whether the service has exceeded the prune threshold.
func shouldPrune(dashboardID, service string) bool {
	pruneMu.Lock()
	defer pruneMu.Unlock()
	if m, ok := pruneCandidates[dashboardID]; ok {
		return m[service] >= PruneThreshold
	}
	return false
}

// clearPruneCandidatesForDashboard removes all candidates for a dashboard.
func clearPruneCandidatesForDashboard(dashboardID string) {
	pruneMu.Lock()
	defer pruneMu.Unlock()
	delete(pruneCandidates, dashboardID)
}
