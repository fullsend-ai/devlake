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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func usageFixture(t *testing.T) File {
	t.Helper()
	file := readFixture(t, "usage/usage.json")
	file.Path = "initiative-rhaistrat/20261118-143311/telemetry/usage.json"
	file.BlobID = "blob"
	file.CollectedAt = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	return file
}

func TestUsageSample(t *testing.T) {
	file := usageFixture(t)
	report := ParseUsage(testConfig(""), file)
	require.Empty(t, report.Diagnostics)
	require.Equal(t, "valid", report.ValidationState)
	require.Equal(t, "2.925981", *report.Totals.CostUSD)
	require.EqualValues(t, 2347524, *report.Totals.CacheReadTokens)
	require.EqualValues(t, 505, *report.Totals.InputTokens)
	require.EqualValues(t, 18133, *report.Totals.OutputTokens)
	require.EqualValues(t, 59, *report.Totals.APIRequests)
	require.EqualValues(t, 68, *report.Totals.ToolCalls)
	require.Nil(t, report.Totals.Turns)
	require.Nil(t, report.Totals.ReasoningTokens)
	require.Equal(t, "18147874639", report.CIJobID)
	require.EqualValues(t, 333152, *report.DurationMs)
	require.Len(t, report.Models, 1)
	require.Equal(t, "claude-opus-4-6", report.Models[0].Name)
	require.JSONEq(t, `{"raw/claude-otel.jsonl.gz":{"bytes":34605,"uncompressed_bytes":719561,"sha256":"55d7b4cb0d956a186a4aa17c465c63cb10c084eb52d0bc5bbdb33d550d498393","removed_attributes":400}}`, report.RawManifest)

	file.CommitSHA = "unrelated-commit"
	file.CollectedAt = file.CollectedAt.Add(time.Hour)
	replayed := ParseUsage(testConfig(""), file)
	require.Equal(t, report.ID, replayed.ID)
	require.Equal(t, report.RunID, replayed.RunID)
	file.BlobID = "correction"
	corrected := ParseUsage(testConfig(""), file)
	require.NotEqual(t, report.ID, corrected.ID)
	require.Equal(t, report.RunID, corrected.RunID)
	file.Body = strings.ReplaceAll(file.Body, "18147874639", "18147874640")
	retry := ParseUsage(testConfig(""), file)
	require.NotEqual(t, report.RunID, retry.RunID)
}

func TestUsageKeepsValidFieldsBesideMalformedValues(t *testing.T) {
	file := usageFixture(t)
	file.Body = strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": NaN`, 1)
	file.Body = strings.Replace(file.Body, `"input_tokens": 505`, `"input_tokens": -3`, 1)
	file.Body = strings.Replace(file.Body, `"output_tokens": 18133`, `"output_tokens": 1.5`, 1)
	file.Body = strings.Replace(file.Body, `"job_id": "18147874639"`, `"job_id": 18147874639`, 1)
	file.Body = strings.Replace(file.Body, `"started_at": "2026-11-18T14:32:56.821Z"`, `"started_at": "2026-99-99T25:99:99Z"`, 1)
	report := ParseUsage(testConfig(""), file)
	require.Equal(t, "partial", report.ValidationState)
	require.GreaterOrEqual(t, len(report.Diagnostics), 5)
	require.Nil(t, report.Totals.CostUSD)
	require.Nil(t, report.Totals.InputTokens)
	require.Nil(t, report.Totals.OutputTokens)
	require.Nil(t, report.StartedAt)
	require.NotNil(t, report.FinishedAt)
	require.Nil(t, report.DurationMs)
	require.Empty(t, report.CIJobID)
	require.Equal(t, "artifact_directory", report.IdentityType)
	require.EqualValues(t, 68, *report.Totals.ToolCalls)
	require.EqualValues(t, 2347524, *report.Totals.CacheReadTokens)
	require.Equal(t, "2.925981", *report.Models[0].CostUSD)
}

func TestUsageNumericBoundaries(t *testing.T) {
	for _, value := range []string{`"42"`, `true`, `[]`, `{}`, `-1`, `0.5`, `9223372036854775808`, `1e1000000000`, `NaN`, `Infinity`, `-Infinity`} {
		t.Run(value, func(t *testing.T) {
			file := usageFixture(t)
			file.Body = strings.Replace(file.Body, `"input_tokens": 505`, `"input_tokens": `+value, 1)
			report := ParseUsage(testConfig(""), file)
			require.Nil(t, report.Totals.InputTokens)
			require.NotEmpty(t, report.Diagnostics)
			require.EqualValues(t, 18133, *report.Totals.OutputTokens)
		})
	}
	for _, value := range []string{"0", "9223372036854775807", "1e3", "42.0"} {
		t.Run("valid-"+value, func(t *testing.T) {
			file := usageFixture(t)
			file.Body = strings.Replace(file.Body, `"input_tokens": 505`, `"input_tokens": `+value, 1)
			require.NotNil(t, ParseUsage(testConfig(""), file).Totals.InputTokens)
		})
	}
	for _, value := range []string{"0.0000001", "1000000000000000000", "-0.1", `"NaN"`} {
		t.Run("cost-"+value, func(t *testing.T) {
			file := usageFixture(t)
			file.Body = strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": `+value, 1)
			require.Nil(t, ParseUsage(testConfig(""), file).Totals.CostUSD)
		})
	}
	file := usageFixture(t)
	file.Body = strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": 999999999999999999.999999`, 1)
	require.Equal(t, "999999999999999999.999999", *ParseUsage(testConfig(""), file).Totals.CostUSD)
}

func TestUsageTimeSemantics(t *testing.T) {
	tests := []struct {
		name, start, finish string
		normalized          bool
		duration            *int64
	}{
		{"offsets", "2026-11-18T10:00:00+02:00", "2026-11-18T09:00:00Z", true, int64ptr(3600000)},
		{"inverted", "2026-11-18T10:00:00Z", "2026-11-18T09:00:00Z", true, nil},
		{"timezone-free", "2026-11-18T10:00:00.123", "2026-11-18T11:00:00.123", false, nil},
		{"invalid", "2026-02-30T10:00:00Z", "2026-11-18T11:00:00Z", false, nil},
		{"invalid-offset", "2026-11-18T10:00:00+24:00", "2026-11-18T11:00:00Z", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := usageFixture(t)
			file.Body = strings.ReplaceAll(file.Body, "2026-11-18T14:32:56.821Z", tt.start)
			file.Body = strings.ReplaceAll(file.Body, "2026-11-18T14:38:29.973Z", tt.finish)
			report := ParseUsage(testConfig(""), file)
			require.Equal(t, tt.normalized, report.StartedAt != nil)
			require.Equal(t, tt.duration, report.DurationMs)
			if tt.name == "timezone-free" {
				require.Equal(t, tt.start, report.StartedAtRaw)
			}
		})
	}
}

func TestUsageFullsendAndMissingData(t *testing.T) {
	file := usageFixture(t)
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(file.Body), &doc))
	doc["source"] = "fullsend-metrics"
	doc["ci"], doc["run_id"], doc["started_at"], doc["finished_at"] = nil, nil, nil, nil
	doc["raw"] = map[string]any{}
	totals := doc["totals"].(map[string]any)
	totals["reasoning_tokens"], totals["turns"] = 50, 4
	totals["api_requests"], totals["user_prompts"] = nil, nil
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	file.Body = string(encoded)
	report := ParseUsage(testConfig(""), file)
	require.Empty(t, report.Diagnostics)
	require.EqualValues(t, 50, *report.Totals.ReasoningTokens)
	require.EqualValues(t, 4, *report.Totals.Turns)
	require.Nil(t, report.Totals.APIRequests)
	require.Nil(t, report.StartedAt)
	require.Equal(t, "artifact_directory", report.IdentityType)

	file.Body = `{"schema":"agentic-telemetry/v1","pipeline":"epic-decomposer","run_id":null,"ci":null,"source":null,"totals":null,"by_model":{},"started_at":null,"finished_at":null,"raw":{}}`
	report = ParseUsage(testConfig(""), file)
	require.False(t, report.HasTotals)
	require.Nil(t, report.Totals.CostUSD)
	require.Empty(t, report.Models)
	require.NotEmpty(t, report.Diagnostics)

	file.Body = `{"schema":"agentic-telemetry/v1","totals":{"tool_calls":0},"by_model":{"good-model":{"output_tokens":12},"bad/model":{"cost_usd":10},"another-model":[]}}`
	report = ParseUsage(testConfig(""), file)
	require.NotEmpty(t, report.Diagnostics)
	require.Len(t, report.Models, 1)
	require.EqualValues(t, 12, *report.Models[0].OutputTokens)
	require.EqualValues(t, 0, *report.Totals.ToolCalls)
	require.Nil(t, report.Totals.InputTokens)
}

func TestUsageUnreadableOrUnsupportedKeepsOnlyProvenance(t *testing.T) {
	for _, body := range []string{"", "{broken", `[]`, `null`, `{"schema":"agentic-telemetry/v2","totals":{"cost_usd":99}}`, `{}`} {
		file := usageFixture(t)
		file.Body = body
		report := ParseUsage(testConfig(""), file)
		require.NotEmpty(t, report.ID)
		require.NotEmpty(t, report.RunID)
		require.NotEmpty(t, report.Diagnostics)
		require.False(t, report.HasTotals)
		require.Nil(t, report.Totals.CostUSD)
		require.Equal(t, file.Path, report.Path)
	}
}

func TestUsageManifestFieldRecovery(t *testing.T) {
	file := usageFixture(t)
	file.Body = strings.Replace(file.Body, `"bytes": 34605`, `"bytes": "too-large"`, 1)
	file.Body = strings.Replace(file.Body, `"removed_attributes": 400`, `"removed_attributes": -1`, 1)
	report := ParseUsage(testConfig(""), file)
	require.Contains(t, report.RawManifest, `"uncompressed_bytes":719561`)
	require.NotContains(t, report.RawManifest, `"bytes"`)
	require.NotContains(t, report.RawManifest, `"removed_attributes"`)
	require.NotEmpty(t, report.Diagnostics)
	require.NotNil(t, report.Totals.CostUSD)
}

func TestUsageNonNullableObjectsAndManifestValues(t *testing.T) {
	file := usageFixture(t)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(file.Body), &doc))
	doc["by_model"] = json.RawMessage(`{"valid-model":null}`)
	doc["raw"] = json.RawMessage(`{"raw/claude-otel.jsonl.gz":{"bytes":null,"uncompressed_bytes":12,"sha256":null}}`)
	encoded, err := json.Marshal(doc)
	require.NoError(t, err)
	file.Body = string(encoded)
	report := ParseUsage(testConfig(""), file)
	require.Len(t, report.Diagnostics, 3)
	require.Empty(t, report.Models)
	require.JSONEq(t, `{"raw/claude-otel.jsonl.gz":{"uncompressed_bytes":12}}`, report.RawManifest)
	require.NotNil(t, report.Totals.CostUSD)
	doc["by_model"], doc["raw"] = json.RawMessage(`null`), json.RawMessage(`null`)
	encoded, err = json.Marshal(doc)
	require.NoError(t, err)
	file.Body = string(encoded)
	require.Len(t, ParseUsage(testConfig(""), file).Diagnostics, 2)
}

func TestRepairNonFiniteJSONDoesNotTouchStrings(t *testing.T) {
	input := `{"name":"NaN Infinity -Infinity \"NaN\"","cost":NaN,"other":Infinity,"negative":-Infinity}`
	got, changed := repairNonFiniteJSON(input)
	require.True(t, changed)
	require.JSONEq(t, `{"name":"NaN Infinity -Infinity \"NaN\"","cost":null,"other":null,"negative":null}`, got)
}

func TestUsageBreakdownMismatchIsDiagnosticOnly(t *testing.T) {
	file := usageFixture(t)
	file.Body = strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": 9`, 1)
	report := ParseUsage(testConfig(""), file)
	require.Equal(t, "9.000000", *report.Totals.CostUSD)
	require.Equal(t, "2.925981", *report.Models[0].CostUSD)
	require.Contains(t, strings.Join(report.Diagnostics, "\n"), "breakdown differs")
	file = usageFixture(t)
	file.Body = strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": 2.925982`, 1)
	require.Empty(t, ParseUsage(testConfig(""), file).Diagnostics, "independent rounding tolerance")
}

func TestUsageSelectionAlongsideResults(t *testing.T) {
	entries := []TreeEntry{
		{Path: "runs/one/pipeline-data.json", Type: "blob"},
		{Path: "runs/one/strat-tasks/item.md", Type: "blob"},
		{Path: "runs/one/telemetry/usage.json", Type: "blob"},
		{Path: "runs/one/telemetry/raw/claude-otel.jsonl.gz", Type: "blob"},
		{Path: "runs/current/telemetry/usage.json", Type: "blob"},
		{Path: "runs/latest/telemetry/usage.json", Type: "blob"},
		{Path: "runs/link/telemetry/usage.json", Type: "blob", Mode: "120000"},
	}
	require.Len(t, SelectPaths(ParserStrat, "", entries), 1)
	require.Len(t, SelectPaths(ParserStrat, "", entries, true), 2)
	require.Len(t, SelectPaths("", "", entries, true), 1)
	require.Empty(t, SelectPaths("", "elsewhere", entries, true))
}

func int64ptr(n int64) *int64 { return &n }
