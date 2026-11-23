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

package models

import (
	"time"

	"github.com/apache/devlake/core/models/common"
)

// AisdlcUsageValues are reported measurements, never inferred from another source.
// Cost is a decimal string in Go and DECIMAL in SQL; nil means unknown.
type AisdlcUsageValues struct {
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

// AisdlcUsageReport is one artifact revision. Query through AisdlcUsageRun for
// current usage; summing report history would count corrected summaries twice.
type AisdlcUsageReport struct {
	common.NoPKModel
	AisdlcUsageValues `gorm:"embedded"`
	Id                string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId      uint64 `gorm:"primaryKey"`
	TenantId          string `gorm:"type:varchar(255)"`
	SourceId          string `gorm:"type:varchar(255)"`
	StationId         string `gorm:"type:varchar(255);index"`
	ProjectPath       string `gorm:"type:varchar(255);index"`
	FilePath          string `gorm:"type:varchar(1024)"`
	BlobId            string `gorm:"type:varchar(64)"`
	CommitSha         string `gorm:"type:varchar(64)"`
	SchemaVersion     string `gorm:"type:varchar(64)"`
	AdapterVersion    string `gorm:"type:varchar(32)"`
	Pipeline          string `gorm:"type:varchar(255)"`
	SourceRunId       string `gorm:"type:varchar(255)"`
	UsageSource       string `gorm:"type:varchar(64)"`
	CIProject         string `gorm:"column:ci_project;type:varchar(255)"`
	CIPipelineID      string `gorm:"column:ci_pipeline_id;type:varchar(20)"`
	CIJobID           string `gorm:"column:ci_job_id;type:varchar(20)"`
	CIJobName         string `gorm:"column:ci_job_name;type:varchar(255)"`
	CIJobURL          string `gorm:"column:ci_job_url;type:varchar(1024)"`
	StartedAtRaw      string `gorm:"type:varchar(64)"`
	FinishedAtRaw     string `gorm:"type:varchar(64)"`
	StartedAt         *time.Time
	FinishedAt        *time.Time
	DurationMs        *int64
	HasTotals         bool
	RawManifest       string
	ValidationState   string `gorm:"type:varchar(32)"`
	Diagnostics       string
	CollectedAt       time.Time
}

func (AisdlcUsageReport) TableName() string { return "_tool_aisdlc_usage_reports" }

type AisdlcUsageModel struct {
	common.NoPKModel
	AisdlcUsageValues `gorm:"embedded"`
	Id                string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId      uint64 `gorm:"primaryKey"`
	ReportId          string `gorm:"type:varchar(64);index"`
	Model             string `gorm:"type:varchar(128)"`
}

func (AisdlcUsageModel) TableName() string { return "_tool_aisdlc_usage_models" }

// AisdlcUsageRun points to one authoritative report. Conflicting copies are
// retained for investigation but must be excluded from usage aggregates.
type AisdlcUsageRun struct {
	common.NoPKModel
	Id           string `gorm:"primaryKey;type:varchar(64)"`
	ConnectionId uint64 `gorm:"primaryKey"`
	ProjectPath  string `gorm:"type:varchar(255);index"`
	ReportId     string `gorm:"type:varchar(64);index"`
	IdentityType string `gorm:"type:varchar(32)"`
	Conflicted   bool
}

func (AisdlcUsageRun) TableName() string { return "_tool_aisdlc_usage_runs" }
