package contextsurface

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

const Version = 1

type Lane string

const (
	LaneBrainMandatory Lane = "brain_mandatory"
	LaneBrainRelevant  Lane = "brain_relevant"
	LaneActive         Lane = "active"
	LaneRecent         Lane = "recent"
	LaneObservation    Lane = "observation"
)

type Item struct {
	ID       string `json:"id"`
	Lane     Lane   `json:"lane"`
	Text     string `json:"text"`
	Source   string `json:"source,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Required bool   `json:"required,omitempty"`
	Trust    string `json:"trust,omitempty"`
}

type Input struct {
	Brain        projectbrain.ContextPacket `json:"brain"`
	Active       []Item                     `json:"active,omitempty"`
	Recent       []Item                     `json:"recent,omitempty"`
	Observations []Item                     `json:"observations,omitempty"`
}

type Budget struct {
	MaxTokens       int `json:"maxTokens"`
	EstimatedTokens int `json:"estimatedTokens"`
	BrainTokens     int `json:"brainTokens"`
	ActiveTokens    int `json:"activeTokens"`
	RecentTokens    int `json:"recentTokens"`
	ObservationTokens int `json:"observationTokens"`
	DroppedItems    int `json:"droppedItems"`
}

type Surface struct {
	Version           int      `json:"version"`
	Fingerprint       string   `json:"fingerprint"`
	BrainFingerprint  string   `json:"brainFingerprint,omitempty"`
	Items             []Item   `json:"items"`
	Budget            Budget   `json:"budget"`
	Truncated         bool     `json:"truncated"`
	MandatoryOverflow bool     `json:"mandatoryOverflow"`
	MutationAllowed   bool     `json:"mutationAllowed"`
	OmittedItemIDs    []string `json:"omittedItemIds,omitempty"`
}

func normalizeItem(item Item, lane Lane) (Item, bool) {
	item.ID = strings.TrimSpace(item.ID)
	item.Text = strings.Join(strings.Fields(item.Text), " ")
	item.Source = strings.TrimSpace(item.Source)
	item.Trust = strings.TrimSpace(item.Trust)
	if item.Lane == "" {
		item.Lane = lane
	}
	if item.ID == "" || item.Text == "" {
		return Item{}, false
	}
	return item, true
}

func itemTokens(item Item) int {
	_, tokens := usage.EstimateTokens(item.Text)
	return tokens
}

func brainItems(packet projectbrain.ContextPacket) []Item {
	out := make([]Item, 0, len(packet.EffectiveRules))
	for _, rule := range packet.EffectiveRules {
		lane := LaneBrainRelevant
		if rule.Required || rule.Lane == "mandatory" {
			lane = LaneBrainMandatory
		}
		item, ok := normalizeItem(Item{
			ID:       "brain:" + rule.ID,
			Lane:     lane,
			Text:     rule.Text,
			Source:   rule.SourcePath,
			Priority: rule.AuthorityRank,
			Required: rule.Required || rule.Lane == "mandatory",
			Trust:    rule.Trust,
		}, lane)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func normalizeLane(items []Item, lane Lane) []Item {
	out := make([]Item, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item, ok := normalizeItem(item, lane)
		if !ok {
			continue
		}
		key := strings.ToLower(item.ID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Required != out[j].Required {
			return out[i].Required
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func appendItem(surface *Surface, item Item, used *int) bool {
	cost := itemTokens(item)
	if cost <= 0 {
		return false
	}
	if *used+cost > surface.Budget.MaxTokens {
		surface.OmittedItemIDs = append(surface.OmittedItemIDs, item.ID)
		surface.Budget.DroppedItems++
		return false
	}
	surface.Items = append(surface.Items, item)
	*used += cost
	switch item.Lane {
	case LaneBrainMandatory, LaneBrainRelevant:
		surface.Budget.BrainTokens += cost
	case LaneActive:
		surface.Budget.ActiveTokens += cost
	case LaneRecent:
		surface.Budget.RecentTokens += cost
	case LaneObservation:
		surface.Budget.ObservationTokens += cost
	}
	return true
}

// Compile builds the model-visible surface from already-resolved Project Brain
// context plus live task projections. It intentionally does not own durable
// history; runtimeevents remains the source of truth and callers provide only
// the active/recent evidence needed for this request.
func Compile(input Input, maxTokens int) Surface {
	if maxTokens <= 0 {
		maxTokens = 16_000
	}
	if maxTokens > 256_000 {
		maxTokens = 256_000
	}
	surface := Surface{
		Version:           Version,
		BrainFingerprint:  input.Brain.Fingerprint,
		Items:             []Item{},
		Budget:            Budget{MaxTokens: maxTokens},
		MandatoryOverflow: input.Brain.MandatoryOverflow,
		MutationAllowed:   input.Brain.MutationAllowed && !input.Brain.MandatoryOverflow,
	}
	used := 0

	brain := brainItems(input.Brain)
	// Brain compiler already established rule authority/order. Required Brain
	// rules remain first and fail closed if the outer model budget cannot hold
	// them, even when the inner Brain character budget succeeded.
	for _, item := range brain {
		if !item.Required {
			continue
		}
		if !appendItem(&surface, item, &used) {
			surface.MandatoryOverflow = true
			surface.MutationAllowed = false
		}
	}

	lanes := [][]Item{
		normalizeLane(input.Active, LaneActive),
		normalizeLane(input.Observations, LaneObservation),
		nil,
		normalizeLane(input.Recent, LaneRecent),
	}
	brainRelevant := make([]Item, 0, len(brain))
	for _, item := range brain {
		if !item.Required {
			brainRelevant = append(brainRelevant, item)
		}
	}
	lanes[2] = brainRelevant

	for _, lane := range lanes {
		for _, item := range lane {
			appendItem(&surface, item, &used)
		}
	}
	surface.Budget.EstimatedTokens = used
	surface.Truncated = surface.Budget.DroppedItems > 0 || input.Brain.Truncated

	parts := []string{input.Brain.Fingerprint}
	for _, item := range surface.Items {
		parts = append(parts, string(item.Lane), item.ID, item.Text)
	}
	for _, id := range surface.OmittedItemIDs {
		parts = append(parts, "omitted", id)
	}
	if surface.MandatoryOverflow {
		parts = append(parts, "mandatory-overflow")
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	surface.Fingerprint = hex.EncodeToString(sum[:])
	return surface
}
