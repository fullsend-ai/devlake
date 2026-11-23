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

package tasks

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/apache/devlake/plugins/aisdlc/tasks/parse"
)

var ExtractUsageMeta = plugin.SubTaskMeta{
	Name:             "extractUsage",
	EntryPoint:       ExtractUsage,
	EnabledByDefault: true,
	Description:      "Normalize agentic-telemetry/v1 usage summaries",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CODE},
	Dependencies:     []*plugin.SubTaskMeta{&CollectFilesMeta},
}

func ExtractUsage(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*AisdlcTaskData)
	cfg := data.Options.ScopeConfig
	if !cfg.UsageEnabled() {
		return nil
	}
	db := taskCtx.GetDal()
	var raws []models.AisdlcRawFile
	if err := db.All(&raws, dal.Where("connection_id = ? AND project_path = ?", data.Connection.ID, data.Options.FullName)); err != nil {
		return err
	}
	config := parse.Config{
		TenantID: cfg.TenantId, SourceID: cfg.SourceId, StationID: cfg.StationId,
		ProjectPath: data.Options.FullName,
	}
	var reports []parse.UsageReport
	for _, file := range filesFromRaw(raws, cfg.PathPrefix) {
		if parse.IsUsagePath(file.Path) {
			reports = append(reports, parse.ParseUsage(config, file))
		}
	}
	runs, diagnostics := currentUsageRuns(reports)
	if len(reports) == 0 {
		diagnostics = append(diagnostics, "no telemetry/usage.json files available in the configured scope")
	}
	for _, report := range latestUsageReports(reports) {
		for _, message := range report.Diagnostics {
			diagnostics = append(diagnostics, parse.UsageDiagnostic(report, message))
		}
	}
	// Producer problems do not fail the subtask. Database failures do: otherwise
	// the next run could silently use stale or partially persisted totals.
	for _, message := range diagnostics {
		taskCtx.GetLogger().Error(nil, "usage: %s; continuing", message)
	}
	if err := saveUsage(db, data.Connection.ID, data.Options.FullName, cfg, reports, runs); err != nil {
		return err
	}
	if len(diagnostics) > 0 {
		return appendCheckpointError(db, data.Connection.ID, data.Options.FullName, checkpointDiagnostics(diagnostics))
	}
	return nil
}

type usageRun struct {
	Report     parse.UsageReport
	Conflicted bool
}

// currentUsageRuns first resolves revisions by artifact path, then deduplicates
// copies of the same producer execution. Collection time is a revision selector,
// never a run timestamp. Removed files keep their last observation time.
func currentUsageRuns(reports []parse.UsageReport) ([]usageRun, []string) {
	byRun := make(map[string]int)
	var runs []usageRun
	var diagnostics []string
	for _, report := range latestUsageReports(reports) {
		if index, exists := byRun[report.RunID]; exists {
			previous := &runs[index]
			if usageFingerprint(previous.Report) != usageFingerprint(report) {
				previous.Conflicted = true
				diagnostics = append(diagnostics, parse.UsageDiagnostic(report, "conflicting copies of one producer run; excluded from aggregates"))
			}
			continue
		}
		byRun[report.RunID] = len(runs)
		runs = append(runs, usageRun{Report: report})
	}
	return runs, diagnostics
}

func latestUsageReports(reports []parse.UsageReport) []parse.UsageReport {
	latest := make(map[string]parse.UsageReport)
	for _, report := range reports {
		previous, exists := latest[report.Path]
		if !exists || report.CollectedAt.After(previous.CollectedAt) ||
			(report.CollectedAt.Equal(previous.CollectedAt) && report.ID > previous.ID) {
			latest[report.Path] = report
		}
	}
	paths := make([]string, 0, len(latest))
	for name := range latest {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	current := make([]parse.UsageReport, 0, len(paths))
	for _, name := range paths {
		current = append(current, latest[name])
	}
	return current
}

func usageFingerprint(report parse.UsageReport) string {
	// Artifact provenance, diagnostics and validation state differ between
	// copies (for example a repaired NaN versus an explicit null), but are not
	// usage. Compare normalized payloads rather than formatting or blob hashes.
	report.File = parse.File{}
	report.ID = ""
	report.Diagnostics = nil
	report.ValidationState = ""
	body, _ := json.Marshal(report)
	return string(body)
}

func saveUsage(db dal.Dal, connectionID uint64, projectPath string, cfg *models.AisdlcScopeConfig, reports []parse.UsageReport, runs []usageRun) errors.Error {
	tx := db.Begin()
	defer func() { _ = tx.Rollback() }()
	stamp := scopeStamp(connectionID, projectPath)
	for _, report := range reports {
		row := &models.AisdlcUsageReport{
			NoPKModel: stamp, AisdlcUsageValues: models.AisdlcUsageValues(report.Totals),
			Id: report.ID, ConnectionId: connectionID,
			TenantId: cfg.TenantId, SourceId: cfg.SourceId, StationId: cfg.StationId,
			ProjectPath: projectPath, FilePath: report.Path, BlobId: report.BlobID, CommitSha: report.CommitSHA,
			SchemaVersion: report.Schema, AdapterVersion: "1", Pipeline: report.Pipeline,
			SourceRunId: report.SourceRunID, UsageSource: report.Source,
			CIProject: report.CIProject, CIPipelineID: report.CIPipelineID,
			CIJobID: report.CIJobID, CIJobName: report.CIJobName, CIJobURL: report.CIJobURL,
			StartedAtRaw: report.StartedAtRaw, FinishedAtRaw: report.FinishedAtRaw,
			StartedAt: report.StartedAt, FinishedAt: report.FinishedAt, DurationMs: report.DurationMs,
			HasTotals: report.HasTotals, RawManifest: report.RawManifest,
			ValidationState: report.ValidationState, Diagnostics: strings.Join(report.Diagnostics, "\n"),
			CollectedAt: report.CollectedAt,
		}
		if err := tx.CreateOrUpdate(row); err != nil {
			return err
		}
		if err := tx.Delete(&models.AisdlcUsageModel{}, dal.Where("connection_id = ? AND report_id = ?", connectionID, report.ID)); err != nil {
			return err
		}
		for _, model := range report.Models {
			row := &models.AisdlcUsageModel{
				NoPKModel: stamp, AisdlcUsageValues: models.AisdlcUsageValues(model.UsageValues),
				Id: parse.ID(report.ID, model.Name), ConnectionId: connectionID, ReportId: report.ID, Model: model.Name,
			}
			if err := tx.Create(row); err != nil {
				return err
			}
		}
	}
	// Rebuild pointers atomically. Correcting CI identifiers or removing models
	// must not leave a previous identity or breakdown contributing to dashboards.
	if err := tx.Delete(&models.AisdlcUsageRun{}, dal.Where("connection_id = ? AND project_path = ?", connectionID, projectPath)); err != nil {
		return err
	}
	for _, run := range runs {
		row := &models.AisdlcUsageRun{
			NoPKModel: stamp, Id: run.Report.RunID, ConnectionId: connectionID, ProjectPath: projectPath,
			ReportId: run.Report.ID, IdentityType: run.Report.IdentityType, Conflicted: run.Conflicted,
		}
		if err := tx.Create(row); err != nil {
			return err
		}
	}
	return tx.Commit()
}
