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

package migrationscripts

import (
	"time"

	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/migrationhelper"
)

// Frozen snapshots: do not import the runtime models into migrations.
type AisdlcUsageValues20261120 struct {
	CostUSD             *string `gorm:"column:cost_usd;type:decimal(24,6)"`
	InputTokens         *int64
	OutputTokens        *int64
	CacheReadTokens     *int64
	CacheCreationTokens *int64
	ReasoningTokens     *int64
	APIRequests         *int64 `gorm:"column:api_requests"`
	ToolCalls           *int64
	Turns               *int64
	UserPrompts         *int64
}

type aisdlcUsageReport20261120 struct {
	AisdlcNoPK20261020
	AisdlcUsageValues20261120 `gorm:"embedded"`
	Id                        string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId              uint64 `gorm:"primaryKey"`
	TenantId                  string `gorm:"type:varchar(255)"`
	SourceId                  string `gorm:"type:varchar(255)"`
	StationId                 string `gorm:"type:varchar(255);index"`
	ProjectPath               string `gorm:"type:varchar(255);index"`
	FilePath                  string `gorm:"type:varchar(1024)"`
	BlobId                    string `gorm:"type:varchar(64)"`
	CommitSha                 string `gorm:"type:varchar(64)"`
	SchemaVersion             string `gorm:"type:varchar(64)"`
	AdapterVersion            string `gorm:"type:varchar(32)"`
	Pipeline                  string `gorm:"type:varchar(255)"`
	SourceRunId               string `gorm:"type:varchar(255)"`
	UsageSource               string `gorm:"type:varchar(64)"`
	CIProject                 string `gorm:"column:ci_project;type:varchar(255)"`
	CIPipelineID              string `gorm:"column:ci_pipeline_id;type:varchar(20)"`
	CIJobID                   string `gorm:"column:ci_job_id;type:varchar(20)"`
	CIJobName                 string `gorm:"column:ci_job_name;type:varchar(255)"`
	CIJobURL                  string `gorm:"column:ci_job_url;type:varchar(1024)"`
	StartedAtRaw              string `gorm:"type:varchar(64)"`
	FinishedAtRaw             string `gorm:"type:varchar(64)"`
	StartedAt                 *time.Time
	FinishedAt                *time.Time
	DurationMs                *int64
	HasTotals                 bool
	RawManifest               string
	ValidationState           string `gorm:"type:varchar(32)"`
	Diagnostics               string
	CollectedAt               time.Time
}

func (aisdlcUsageReport20261120) TableName() string { return "_tool_aisdlc_usage_reports" }

type aisdlcUsageModel20261120 struct {
	AisdlcNoPK20261020
	AisdlcUsageValues20261120 `gorm:"embedded"`
	Id                        string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId              uint64 `gorm:"primaryKey"`
	ReportId                  string `gorm:"type:varchar(64);index"`
	Model                     string `gorm:"type:varchar(128)"`
}

func (aisdlcUsageModel20261120) TableName() string { return "_tool_aisdlc_usage_models" }

type aisdlcUsageRun20261120 struct {
	AisdlcNoPK20261020
	Id           string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId uint64 `gorm:"primaryKey"`
	ProjectPath  string `gorm:"type:varchar(255);index"`
	ReportId     string `gorm:"type:varchar(64);index"`
	IdentityType string `gorm:"type:varchar(32)"`
	Conflicted   bool
}

func (aisdlcUsageRun20261120) TableName() string { return "_tool_aisdlc_usage_runs" }

type aisdlcUsageConfig20261120 struct {
	ID        uint64 `gorm:"primaryKey"`
	UsageMode string `gorm:"type:varchar(32)"`
}

func (aisdlcUsageConfig20261120) TableName() string { return "_tool_aisdlc_scope_configs" }

type addUsage struct{}

var _ plugin.MigrationScript = (*addUsage)(nil)

func (*addUsage) Up(res context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(res,
		&aisdlcUsageReport20261120{}, &aisdlcUsageModel20261120{},
		&aisdlcUsageRun20261120{}, &aisdlcUsageConfig20261120{})
}

func (*addUsage) Version() uint64 { return 20261120000001 }
func (*addUsage) Name() string    { return "aisdlc run usage summaries" }
