/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const UsageSchema = "agentic-telemetry/v1"

// UsageValues preserves unknown measurements as nil. Decimal cost is represented
// as a string to avoid losing precision between JSON and DECIMAL(24,6) storage.
type UsageValues struct {
	CostUSD             *string
	InputTokens         *int64
	OutputTokens        *int64
	CacheReadTokens     *int64
	CacheCreationTokens *int64
	ReasoningTokens     *int64
	APIRequests         *int64
	ToolCalls           *int64
	Turns               *int64
	UserPrompts         *int64
}

type UsageModel struct {
	Name string
	UsageValues
}

type UsageReport struct {
	File
	ID              string
	RunID           string
	IdentityType    string
	Schema          string
	Pipeline        string
	SourceRunID     string
	Source          string
	CIProject       string
	CIPipelineID    string
	CIJobID         string
	CIJobName       string
	CIJobURL        string
	StartedAtRaw    string
	FinishedAtRaw   string
	StartedAt       *time.Time
	FinishedAt      *time.Time
	DurationMs      *int64
	HasTotals       bool
	Totals          UsageValues
	Models          []UsageModel
	RawManifest     string
	ValidationState string
	Diagnostics     []string
}

var (
	usageModelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}(@[0-9]{8})?(:[0-9]{1,3})?$`)
	usageCIID      = regexp.MustCompile(`^[0-9]{1,20}$`)
	usageProject   = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_./-]{0,254}$`)
	usageJobURL    = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:[0-9]+)?/[A-Za-z0-9_./-]+/-/jobs/[0-9]+$`)
	usageSHA       = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	usageTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-](0[0-9]|1[0-9]|2[0-3]):[0-5][0-9])?$`)
)

// ParseUsage is deliberately field-wise: a malformed measurement never discards
// valid sibling measurements. Diagnostics describe fields, never producer values.
// Unreadable JSON and unknown schema versions retain only provenance.
func ParseUsage(cfg Config, file File) UsageReport {
	r := UsageReport{File: file, RawManifest: "{}", ValidationState: "valid"}
	r.ID = ID(cfg.TenantID, cfg.SourceID, cfg.ProjectPath, file.Path, file.BlobID, "usage")
	body, repaired := repairNonFiniteJSON(file.Body)
	if repaired {
		r.problem("$", "non-finite JSON numbers replaced with null")
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &doc); err != nil || doc == nil {
		r.problem("$", "expected a JSON object")
		r.ValidationState = "invalid"
		r.setIdentity(cfg)
		return r
	}
	r.Schema = r.text(doc, "schema", "", 64, true)
	if r.Schema != UsageSchema {
		r.problem("schema", "missing or unsupported schema version")
		r.ValidationState = "unsupported"
		r.setIdentity(cfg)
		return r
	}
	r.Pipeline = r.text(doc, "pipeline", "", 255, true)
	r.SourceRunID = r.text(doc, "run_id", "", 255, false)
	r.Source = r.text(doc, "source", "", 64, false)
	if r.Source != "" && r.Source != "claude-otel" && r.Source != "fullsend-metrics" {
		r.problem("source", "unsupported usage source")
		r.Source = ""
	}
	if ci := r.object(doc, "ci", ""); ci != nil {
		r.CIProject = r.checkedText(ci, "project", "ci.", 255, usageProject)
		r.CIPipelineID = r.checkedText(ci, "pipeline_id", "ci.", 20, usageCIID)
		r.CIJobID = r.checkedText(ci, "job_id", "ci.", 20, usageCIID)
		r.CIJobName = r.text(ci, "job_name", "ci.", 255, false)
		r.CIJobURL = r.checkedText(ci, "job_url", "ci.", 1024, usageJobURL)
	}
	if totals := r.object(doc, "totals", ""); totals != nil {
		r.HasTotals = true
		r.Totals = r.values(totals, "totals.", true)
	}
	if models := r.requiredObject(doc, "by_model", ""); models != nil {
		for _, name := range sortedKeys(models) {
			if !usageModelName.MatchString(name) {
				r.problem("by_model", "invalid model name; entry skipped")
				continue
			}
			if entry := r.requiredObject(models, name, "by_model."); entry != nil {
				r.Models = append(r.Models, UsageModel{Name: name, UsageValues: r.values(entry, "by_model."+name+".", false)})
			}
		}
	}
	r.StartedAtRaw, r.StartedAt = r.timestamp(doc, "started_at")
	r.FinishedAtRaw, r.FinishedAt = r.timestamp(doc, "finished_at")
	if r.StartedAt != nil && r.FinishedAt != nil {
		if r.FinishedAt.Before(*r.StartedAt) {
			r.problem("finished_at", "precedes started_at; duration unavailable")
		} else {
			// Unix milliseconds avoid time.Duration's ~290 year overflow.
			duration := r.FinishedAt.UnixMilli() - r.StartedAt.UnixMilli()
			r.DurationMs = &duration
		}
	}
	r.manifest(doc)
	r.checkBreakdown()
	if !r.HasTotals && len(r.Models) == 0 {
		r.problem("totals", "no usage measurements available")
	}
	r.setIdentity(cfg)
	return r
}

// Check only measurements known for every listed model. A mismatch is useful
// evidence of incomplete producer coverage, not a reason to rewrite totals.
func (r *UsageReport) checkBreakdown() {
	if !r.HasTotals || len(r.Models) == 0 {
		return
	}
	counts := []struct {
		key string
		get func(UsageValues) *int64
	}{
		{"input_tokens", func(v UsageValues) *int64 { return v.InputTokens }},
		{"output_tokens", func(v UsageValues) *int64 { return v.OutputTokens }},
		{"cache_read_tokens", func(v UsageValues) *int64 { return v.CacheReadTokens }},
		{"cache_creation_tokens", func(v UsageValues) *int64 { return v.CacheCreationTokens }},
		{"reasoning_tokens", func(v UsageValues) *int64 { return v.ReasoningTokens }},
		{"api_requests", func(v UsageValues) *int64 { return v.APIRequests }},
	}
	for _, field := range counts {
		total := field.get(r.Totals)
		if total == nil {
			continue
		}
		sum, complete := new(big.Int), true
		for _, model := range r.Models {
			value := field.get(model.UsageValues)
			if value == nil {
				complete = false
				break
			}
			sum.Add(sum, big.NewInt(*value))
		}
		if complete && sum.Cmp(big.NewInt(*total)) != 0 {
			r.problem("by_model."+field.key, "breakdown differs from reported total; both retained")
		}
	}
	if r.Totals.CostUSD == nil {
		return
	}
	total, _ := new(big.Rat).SetString(*r.Totals.CostUSD)
	sum := new(big.Rat)
	for _, model := range r.Models {
		if model.CostUSD == nil {
			return
		}
		cost, _ := new(big.Rat).SetString(*model.CostUSD)
		sum.Add(sum, cost)
	}
	delta := new(big.Rat).Abs(new(big.Rat).Sub(sum, total))
	// Each independently rounded model and the total can differ by half a
	// micro-dollar. Account for that without using floating point.
	tolerance := big.NewRat(int64(len(r.Models))+1, 2000000)
	if delta.Cmp(tolerance) > 0 {
		r.problem("by_model.cost_usd", "breakdown differs from reported total beyond rounding tolerance; both retained")
	}
}

func (r *UsageReport) problem(field, message string) {
	r.Diagnostics = append(r.Diagnostics, field+": "+message)
	r.ValidationState = "partial"
}

func (r *UsageReport) field(doc map[string]json.RawMessage, key, prefix string) json.RawMessage {
	value, ok := doc[key]
	if !ok {
		r.problem(prefix+key, "missing field")
	}
	return value
}

func isNull(value json.RawMessage) bool {
	return len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func (r *UsageReport) text(doc map[string]json.RawMessage, key, prefix string, max int, required bool) string {
	value := r.field(doc, key, prefix)
	if isNull(value) {
		if required && len(value) != 0 {
			r.problem(prefix+key, "required string is null")
		}
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) != nil || text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > max ||
		strings.ContainsAny(text, "\x00\r\n") {
		r.problem(prefix+key, "expected a nonempty bounded string")
		return ""
	}
	return text
}

func (r *UsageReport) checkedText(doc map[string]json.RawMessage, key, prefix string, max int, pattern *regexp.Regexp) string {
	text := r.text(doc, key, prefix, max, false)
	if text != "" && !pattern.MatchString(text) {
		r.problem(prefix+key, "invalid format")
		return ""
	}
	return text
}

func (r *UsageReport) object(doc map[string]json.RawMessage, key, prefix string) map[string]json.RawMessage {
	value := r.field(doc, key, prefix)
	if isNull(value) {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) != nil || object == nil {
		r.problem(prefix+key, "expected an object or null")
		return nil
	}
	return object
}

func (r *UsageReport) requiredObject(doc map[string]json.RawMessage, key, prefix string) map[string]json.RawMessage {
	object := r.object(doc, key, prefix)
	if value, exists := doc[key]; exists && isNull(value) {
		r.problem(prefix+key, "expected an object, not null")
	}
	return object
}

func (r *UsageReport) number(doc map[string]json.RawMessage, key, prefix string) *big.Rat {
	value := r.field(doc, key, prefix)
	if isNull(value) {
		return nil
	}
	// RawMessage has already passed JSON syntax validation. Restrict length to
	// bound big.Rat allocation, especially for producer-supplied exponents.
	if len(value) > 128 || (value[0] != '-' && (value[0] < '0' || value[0] > '9')) {
		r.problem(prefix+key, "expected a nonnegative JSON number or null")
		return nil
	}
	var n json.Number
	if json.Unmarshal(value, &n) != nil {
		r.problem(prefix+key, "expected a JSON number")
		return nil
	}
	if i := strings.IndexAny(string(n), "eE"); i >= 0 {
		exponent := new(big.Int)
		if _, ok := exponent.SetString(string(n)[i+1:], 10); !ok || !exponent.IsInt64() || exponent.Int64() > 24 || exponent.Int64() < -24 {
			r.problem(prefix+key, "numeric exponent out of range")
			return nil
		}
	}
	number, ok := new(big.Rat).SetString(string(n))
	if !ok || number.Sign() < 0 {
		r.problem(prefix+key, "expected a finite nonnegative number")
		return nil
	}
	return number
}

func (r *UsageReport) count(doc map[string]json.RawMessage, key, prefix string) *int64 {
	n := r.number(doc, key, prefix)
	if n == nil {
		return nil
	}
	if !n.IsInt() || !n.Num().IsInt64() {
		r.problem(prefix+key, "expected a nonnegative 64-bit integer")
		return nil
	}
	value := n.Num().Int64()
	return &value
}

func (r *UsageReport) values(doc map[string]json.RawMessage, prefix string, totals bool) UsageValues {
	v := UsageValues{
		InputTokens: r.count(doc, "input_tokens", prefix), OutputTokens: r.count(doc, "output_tokens", prefix),
		CacheReadTokens: r.count(doc, "cache_read_tokens", prefix), CacheCreationTokens: r.count(doc, "cache_creation_tokens", prefix),
		ReasoningTokens: r.count(doc, "reasoning_tokens", prefix), APIRequests: r.count(doc, "api_requests", prefix),
	}
	if cost := r.number(doc, "cost_usd", prefix); cost != nil {
		value := cost.FloatString(6)
		rounded, _ := new(big.Rat).SetString(value)
		if len(strings.SplitN(value, ".", 2)[0]) > 18 || rounded.Cmp(cost) != 0 {
			r.problem(prefix+"cost_usd", "cost must fit DECIMAL(24,6) without rounding")
		} else {
			v.CostUSD = &value
		}
	}
	if totals {
		v.ToolCalls = r.count(doc, "tool_calls", prefix)
		v.Turns = r.count(doc, "turns", prefix)
		v.UserPrompts = r.count(doc, "user_prompts", prefix)
	}
	return v
}

func (r *UsageReport) timestamp(doc map[string]json.RawMessage, key string) (string, *time.Time) {
	text := r.text(doc, key, "", 64, false)
	if text == "" {
		return "", nil
	}
	if usageTimestamp.MatchString(text) {
		if stamp, err := time.Parse(time.RFC3339Nano, text); err == nil && stamp.Year() >= 1000 && stamp.UTC().Year() >= 1000 && stamp.UTC().Year() <= 9999 {
			utc := stamp.UTC()
			return text, &utc
		}
		if stamp, err := time.Parse("2006-01-02T15:04:05", text); err == nil && stamp.Year() >= 1000 {
			r.problem(key, "timezone unspecified; normalized timestamp unavailable")
			return text, nil
		}
	}
	r.problem(key, "invalid timestamp")
	return "", nil
}

func (r *UsageReport) manifest(doc map[string]json.RawMessage) {
	manifest := r.requiredObject(doc, "raw", "")
	cleaned := make(map[string]map[string]any)
	for _, name := range sortedKeys(manifest) {
		if name != "raw/claude-otel.jsonl.gz" && name != "raw/fullsend-metrics.json.gz" && name != "raw/fullsend-telemetry.jsonl.gz" {
			r.problem("raw", "unsupported artifact path; entry skipped")
			continue
		}
		entry := r.requiredObject(manifest, name, "raw.")
		if entry == nil {
			continue
		}
		prefix := "raw." + name + "."
		fields := make(map[string]any)
		for _, key := range []string{"bytes", "uncompressed_bytes", "removed_attributes"} {
			if key == "removed_attributes" {
				if _, exists := entry[key]; !exists {
					continue
				}
			}
			if value := r.count(entry, key, prefix); value != nil {
				fields[key] = *value
			} else if raw, exists := entry[key]; exists && isNull(raw) {
				r.problem(prefix+key, "manifest measurement cannot be null")
			}
		}
		if sha := r.checkedText(entry, "sha256", prefix, 64, usageSHA); sha != "" {
			fields["sha256"] = sha
		} else if raw, exists := entry["sha256"]; exists && isNull(raw) {
			r.problem(prefix+"sha256", "manifest checksum cannot be null")
		}
		cleaned[name] = fields
	}
	encoded, _ := json.Marshal(cleaned)
	r.RawManifest = string(encoded)
}

func (r *UsageReport) setIdentity(cfg Config) {
	parts := []string{cfg.TenantID, cfg.SourceID, cfg.ProjectPath, "usage-run"}
	if r.CIProject != "" && r.CIJobID != "" {
		r.IdentityType = "ci_job"
		parts = append(parts, r.IdentityType, r.CIProject, r.CIJobID, r.SourceRunID)
	} else {
		// A timestamp alone is not globally unique. The directory is stable even
		// when a corrected report fills in a previously missing run identifier.
		r.IdentityType = "artifact_directory"
		parts = append(parts, r.IdentityType, path.Dir(path.Dir(r.Path)))
	}
	r.RunID = ID(parts...)
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// repairNonFiniteJSON accepts only the three bare constants emitted by Python's
// default JSON encoder. Strings are untouched; other syntax errors are not
// guessed at. This salvages valid siblings of NaN/Infinity measurements.
func repairNonFiniteJSON(input string) (string, bool) {
	var out strings.Builder
	quoted, escaped, changed := false, false, false
	for i := 0; i < len(input); {
		c := input[i]
		if !quoted && (i == 0 || strings.ContainsRune(" \t\r\n[:,", rune(input[i-1]))) {
			replaced := false
			for _, token := range []string{"NaN", "Infinity", "-Infinity"} {
				end := i + len(token)
				if strings.HasPrefix(input[i:], token) && (end == len(input) || strings.ContainsRune(" \t\r\n,}]", rune(input[end]))) {
					out.WriteString("null")
					i = end
					changed, replaced = true, true
					break
				}
			}
			if replaced {
				continue
			}
		}
		out.WriteByte(c)
		i++
		if escaped {
			escaped = false
		} else if quoted && c == '\\' {
			escaped = true
		} else if c == '"' {
			quoted = !quoted
		}
	}
	return out.String(), changed
}

// UsageDiagnostic prefixes messages with artifact provenance, not its contents.
func UsageDiagnostic(report UsageReport, message string) string {
	return fmt.Sprintf("%s (blob %s): %s", report.Path, report.BlobID, message)
}
