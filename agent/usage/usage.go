package usage

import (
	"math"
	"sort"
	"sync"
	"time"
)

type TokenKind string

const (
	KindInput      TokenKind = "input"
	KindOutput     TokenKind = "output"
	KindReasoning  TokenKind = "reasoning"
	KindCacheRead  TokenKind = "cache_read"
	KindCacheWrite TokenKind = "cache_write"
)

type TokenItem struct {
	Kind  TokenKind `json:"kind"`
	Count int       `json:"count"`
}

type TokenItems []TokenItem

func (t TokenItems) Count(kind TokenKind) int {
	for _, item := range t {
		if item.Kind == kind {
			return item.Count
		}
	}
	return 0
}

func (t TokenItems) TotalInput() int {
	return t.Count(KindInput) + t.Count(KindCacheRead) + t.Count(KindCacheWrite)
}

func (t TokenItems) TotalOutput() int {
	return t.Count(KindOutput) + t.Count(KindReasoning)
}

func (t TokenItems) Total() int {
	return t.TotalInput() + t.TotalOutput()
}

func (t TokenItems) NonZero() TokenItems {
	var result TokenItems
	for _, item := range t {
		if item.Count > 0 {
			result = append(result, item)
		}
	}
	return result
}

type Cost struct {
	Total      float64 `json:"total"`
	Input      float64 `json:"input,omitempty"`
	Output     float64 `json:"output,omitempty"`
	Reasoning  float64 `json:"reasoning,omitempty"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
	Source     string  `json:"source,omitempty"`
}

func (c Cost) IsZero() bool {
	return c.Source == "" && c.Total == 0
}

type Dims struct {
	Provider  string            `json:"provider,omitempty"`
	Model     string            `json:"model,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	TurnID    string            `json:"turn_id,omitempty"`
	SessionID string            `json:"session_id,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type Record struct {
	Tokens     TokenItems     `json:"tokens"`
	Cost       Cost           `json:"cost"`
	Dims       Dims           `json:"dims"`
	IsEstimate bool           `json:"is_estimate,omitempty"`
	RecordedAt time.Time      `json:"recorded_at"`
	Source     string         `json:"source,omitempty"`
	Encoder    string         `json:"encoder,omitempty"`
	Extras     map[string]any `json:"extras,omitempty"`
}

type Drift struct {
	Dims           Dims
	EstimatedInput int
	ActualInput    int
	InputDelta     int
	InputPct       float64
	Estimate       Record
	Actual         Record
}

type DriftStats struct {
	N       int
	MinPct  float64
	MaxPct  float64
	MeanPct float64
	P50Pct  float64
	P95Pct  float64
}

func ComputeDrift(estimate, actual *Record) *Drift {
	if estimate == nil || actual == nil || !estimate.IsEstimate {
		return nil
	}

	estInput := estimate.Tokens.TotalInput()
	actInput := actual.Tokens.TotalInput()
	delta := actInput - estInput

	pct := math.NaN()
	if estInput > 0 {
		pct = float64(delta) / float64(estInput) * 100.0
	}

	return &Drift{
		Dims:           actual.Dims,
		EstimatedInput: estInput,
		ActualInput:    actInput,
		InputDelta:     delta,
		InputPct:       pct,
		Estimate:       *estimate,
		Actual:         *actual,
	}
}

type CostCalculator interface {
	Calculate(provider, model string, tokens TokenItems) (Cost, bool)
}

type CostCalculatorFunc func(provider, model string, tokens TokenItems) (Cost, bool)

func (f CostCalculatorFunc) Calculate(p, m string, t TokenItems) (Cost, bool) { return f(p, m, t) }

type Budget struct{}

func (b Budget) Exceeded(r Record) bool { return false }

type Tracker struct {
	mu         sync.Mutex
	records    []Record
	budget     Budget
	calculator CostCalculator
	sessionID  string
}

type TrackerOption func(*Tracker)

func WithBudget(b Budget) TrackerOption {
	return func(t *Tracker) { t.budget = b }
}

func WithSessionID(id string) TrackerOption {
	return func(t *Tracker) { t.sessionID = id }
}

func WithCostCalculator(c CostCalculator) TrackerOption {
	return func(t *Tracker) { t.calculator = c }
}

func NewTracker(opts ...TrackerOption) *Tracker {
	t := &Tracker{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *Tracker) Record(r Record) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if r.Cost.IsZero() && t.calculator != nil {
		if cost, ok := t.calculator.Calculate(r.Dims.Provider, r.Dims.Model, r.Tokens); ok {
			r.Cost = cost
		}
	}

	t.records = append(t.records, r)
}

func (t *Tracker) Records() []Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]Record, len(t.records))
	copy(result, t.records)
	return result
}

func (t *Tracker) Aggregate() Record {
	t.mu.Lock()
	defer t.mu.Unlock()

	var agg Record
	agg.RecordedAt = time.Now()

	counts := make(map[TokenKind]int)
	var totalCost Cost

	for _, r := range t.records {
		if r.IsEstimate {
			continue
		}
		for _, item := range r.Tokens {
			counts[item.Kind] += item.Count
		}
		totalCost.Total += r.Cost.Total
		totalCost.Input += r.Cost.Input
		totalCost.Output += r.Cost.Output
		totalCost.Reasoning += r.Cost.Reasoning
		totalCost.CacheRead += r.Cost.CacheRead
		totalCost.CacheWrite += r.Cost.CacheWrite
		if totalCost.Source == "" {
			totalCost.Source = r.Cost.Source
		}
	}

	for kind, count := range counts {
		if count > 0 {
			agg.Tokens = append(agg.Tokens, TokenItem{Kind: kind, Count: count})
		}
	}
	agg.Cost = totalCost

	return agg
}

type FilterFunc func(Record) bool

func (t *Tracker) Filter(fs ...FilterFunc) []Record {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result []Record
outer:
	for _, r := range t.records {
		for _, f := range fs {
			if !f(r) {
				continue outer
			}
		}
		result = append(result, r)
	}
	return result
}

func (t *Tracker) WithinBudget() bool {
	agg := t.Aggregate()
	return !t.budget.Exceeded(agg)
}

func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = nil
}

func (t *Tracker) Drift(requestID string) (*Drift, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var estimate, actual *Record
	for i := range t.records {
		r := &t.records[i]
		if r.Dims.RequestID != requestID {
			continue
		}
		if r.IsEstimate && r.Dims.Labels == nil && estimate == nil {
			estimate = r
		}
		if !r.IsEstimate && actual == nil {
			actual = r
		}
		if estimate != nil && actual != nil {
			break
		}
	}

	if estimate == nil || actual == nil {
		return nil, false
	}

	return ComputeDrift(estimate, actual), true
}

func (t *Tracker) Drifts() []Drift {
	t.mu.Lock()
	defer t.mu.Unlock()

	type pair struct {
		estimate Record
		actual   Record
	}

	pairsByID := make(map[string]*pair)

	for _, r := range t.records {
		if r.Dims.RequestID == "" {
			continue
		}

		p, ok := pairsByID[r.Dims.RequestID]
		if !ok {
			p = &pair{}
			pairsByID[r.Dims.RequestID] = p
		}

		if r.IsEstimate && r.Dims.Labels == nil {
			p.estimate = r
		}
		if !r.IsEstimate {
			p.actual = r
		}
	}

	var drifts []Drift
	for _, p := range pairsByID {
		if !p.estimate.RecordedAt.IsZero() && !p.actual.RecordedAt.IsZero() {
			if d := ComputeDrift(&p.estimate, &p.actual); d != nil {
				drifts = append(drifts, *d)
			}
		}
	}

	sort.Slice(drifts, func(i, j int) bool {
		return drifts[i].Actual.RecordedAt.Before(drifts[j].Actual.RecordedAt)
	})

	return drifts
}

func (t *Tracker) DriftStats() DriftStats {
	drifts := t.Drifts()
	if len(drifts) == 0 {
		return DriftStats{}
	}

	pcts := make([]float64, 0, len(drifts))
	var sum float64
	var min, max float64 = math.MaxFloat64, -math.MaxFloat64

	for _, d := range drifts {
		if !math.IsNaN(d.InputPct) {
			pcts = append(pcts, d.InputPct)
			sum += d.InputPct
			if d.InputPct < min {
				min = d.InputPct
			}
			if d.InputPct > max {
				max = d.InputPct
			}
		}
	}

	if len(pcts) == 0 {
		return DriftStats{N: len(drifts)}
	}

	sort.Float64s(pcts)
	mean := sum / float64(len(pcts))
	p50 := pcts[len(pcts)/2]
	p95Idx := int(float64(len(pcts)) * 0.95)
	if p95Idx >= len(pcts) {
		p95Idx = len(pcts) - 1
	}
	p95 := pcts[p95Idx]

	return DriftStats{
		N:       len(drifts),
		MinPct:  min,
		MaxPct:  max,
		MeanPct: mean,
		P50Pct:  p50,
		P95Pct:  p95,
	}
}

func ByProvider(name string) FilterFunc {
	return func(r Record) bool { return r.Dims.Provider == name }
}

func ByModel(model string) FilterFunc {
	return func(r Record) bool { return r.Dims.Model == model }
}

func ByTurnID(id string) FilterFunc {
	return func(r Record) bool { return r.Dims.TurnID == id }
}

func BySessionID(id string) FilterFunc {
	return func(r Record) bool { return r.Dims.SessionID == id }
}

func EstimatesOnly() FilterFunc {
	return func(r Record) bool { return r.IsEstimate }
}

func ExcludeEstimates() FilterFunc {
	return func(r Record) bool { return !r.IsEstimate }
}

func Since(t time.Time) FilterFunc {
	return func(r Record) bool { return r.RecordedAt.After(t) }
}

func ByLabel(key, value string) FilterFunc {
	return func(r Record) bool {
		if r.Dims.Labels == nil {
			return false
		}
		return r.Dims.Labels[key] == value
	}
}
