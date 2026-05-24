package sync

import (
	"fmt"
	"integration-project-ehb/controlroom/pkg/logger"
	"strings"
)

// ClassifyAndPrunePanels classifies panels into static and dynamic groups based
// on title prefixes provided in prefixKinds (map[prefix]kind). It applies the
// prune grace policy using the prune helpers and returns:
// - staticPanels: panels to keep as static
// - dynamicPanels: map[kind]map[service]panel
// - changed: whether any prune decision caused a change (panels removed)
func ClassifyAndPrunePanels(dashboardID string, panels []map[string]any, refs []map[string]any, titleMap map[string]string, prefixKinds map[string]string, activeServiceSet map[string]bool) ([]map[string]any, map[string]map[string]map[string]any, bool) {
	staticPanels := make([]map[string]any, 0, len(panels))
	dynamicPanels := map[string]map[string]map[string]any{}
	changed := false

	// Prepare dynamic maps
	for _, kind := range prefixKinds {
		if _, ok := dynamicPanels[kind]; !ok {
			dynamicPanels[kind] = map[string]map[string]any{}
		}
	}

	skipPruneOnEmpty := len(activeServiceSet) == 0

	for _, p := range panels {
		pType, _ := p["type"].(string)

		// allow all types; classification is by title prefix
		if pType == "" {
			staticPanels = append(staticPanels, p)
			continue
		}

		title := getPanelTitle(p, refs, titleMap)
		if title == "" {
			staticPanels = append(staticPanels, p)
			continue
		}

		matched := false
		for prefix, kind := range prefixKinds {
			if after, ok := strings.CutPrefix(title, prefix); ok {
				svc := after
				matched = true
				if activeServiceSet[svc] {
					dynamicPanels[kind][svc] = p
					resetPruneCandidate(dashboardID, svc)
				} else {
					if skipPruneOnEmpty {
						staticPanels = append(staticPanels, p)
						logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: preserving %s panel for %s due to empty discovery", kind, svc)))
						break
					}

					miss := incrPruneCandidate(dashboardID, svc)
					if shouldPrune(dashboardID, svc) {
						logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: pruning stale %s panel for service %s (misses=%d)", kind, svc, miss)))
						changed = true
						break
					}

					staticPanels = append(staticPanels, p)
					logger.Log(logger.NewMessage(logger.DEBUG, logger.CONTROLROOM, fmt.Sprintf("dashboard sync: deferring prune for %s panel %s (misses=%d/%d)", kind, svc, miss, PruneThreshold)))
				}
				break
			}
		}

		if !matched {
			staticPanels = append(staticPanels, p)
		}
	}

	return staticPanels, dynamicPanels, changed
}
