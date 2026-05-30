package pipeline

// timelinePanelsByIndex reads cfg.Params["timeline"]["panels"] and returns the
// panel-level overrides keyed by the source image panel index. Empty map when
// no timeline was supplied.
func timelinePanelsByIndex(params map[string]any) map[int]map[string]any {
	out := map[int]map[string]any{}
	tl, ok := params["timeline"].(map[string]any)
	if !ok {
		return out
	}
	panels, ok := tl["panels"].([]any)
	if !ok {
		return out
	}
	for _, p := range panels {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		idxF, _ := m["image_panel_index"].(float64)
		out[int(idxF)] = m
	}
	return out
}

// timelineOrder reads cfg.Params["timeline"]["panels"] in order and returns
// the image_panel_index sequence. Used to reorder panels per the editor.
func timelineOrder(params map[string]any) []int {
	tl, ok := params["timeline"].(map[string]any)
	if !ok {
		return nil
	}
	panels, ok := tl["panels"].([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(panels))
	for _, p := range panels {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if idx, ok := m["image_panel_index"].(float64); ok {
			out = append(out, int(idx))
		}
	}
	return out
}

// reorderRefs rearranges refs to match the timeline's order. Panels not in
// order are dropped (intentional — the editor explicitly selects what plays).
func reorderRefs(refs []AssemblePanelRef, order []int) []AssemblePanelRef {
	byIdx := map[int]AssemblePanelRef{}
	for _, r := range refs {
		byIdx[r.Index] = r
	}
	out := make([]AssemblePanelRef, 0, len(order))
	for _, idx := range order {
		if r, ok := byIdx[idx]; ok {
			out = append(out, r)
		}
	}
	return out
}
