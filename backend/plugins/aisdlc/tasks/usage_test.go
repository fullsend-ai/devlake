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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/apache/devlake/core/errors"
	mockdal "github.com/apache/devlake/mocks/core/dal"
	mocklog "github.com/apache/devlake/mocks/core/log"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/apache/devlake/plugins/aisdlc/tasks/parse"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func usageTestFile(t *testing.T) parse.File {
	t.Helper()
	body, err := os.ReadFile("testdata/usage/usage.json")
	require.NoError(t, err)
	return parse.File{
		Path: "run/telemetry/usage.json", BlobID: "first", Body: string(body),
		CollectedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
	}
}

func TestCurrentUsageRevisionAndRetry(t *testing.T) {
	cfg := parse.Config{SourceID: "source", TenantID: "tenant", ProjectPath: "group/results"}
	file := usageTestFile(t)
	first := parse.ParseUsage(cfg, file)
	file.BlobID = "corrected"
	file.CollectedAt = file.CollectedAt.Add(time.Hour)
	file.Body = strings.ReplaceAll(file.Body, "2.925981", "3.000001")
	corrected := parse.ParseUsage(cfg, file)
	for _, reports := range [][]parse.UsageReport{{first, corrected}, {corrected, first}, {corrected, first, corrected}} {
		runs, diagnostics := currentUsageRuns(reports)
		require.Empty(t, diagnostics)
		require.Len(t, runs, 1)
		require.Equal(t, corrected.ID, runs[0].Report.ID)
	}
	file.Path = "retry/telemetry/usage.json"
	file.Body = strings.ReplaceAll(file.Body, "18147874639", "18147874640")
	retry := parse.ParseUsage(cfg, file)
	runs, diagnostics := currentUsageRuns([]parse.UsageReport{first, corrected, retry})
	require.Empty(t, diagnostics)
	require.Len(t, runs, 2)
}

func TestCurrentUsageCopiesAndConflicts(t *testing.T) {
	cfg := parse.Config{SourceID: "source"}
	file := usageTestFile(t)
	first := parse.ParseUsage(cfg, file)
	file.Path = "copy/telemetry/usage.json"
	file.BlobID = "copy"
	file.Body = strings.ReplaceAll(file.Body, "\n", "")
	copy := parse.ParseUsage(cfg, file)
	runs, diagnostics := currentUsageRuns([]parse.UsageReport{first, copy})
	require.Empty(t, diagnostics)
	require.Len(t, runs, 1)
	require.False(t, runs[0].Conflicted)
	file.Body = strings.ReplaceAll(file.Body, "2.925981", "99")
	conflict := parse.ParseUsage(cfg, file)
	runs, diagnostics = currentUsageRuns([]parse.UsageReport{first, conflict})
	require.Len(t, runs, 1)
	require.True(t, runs[0].Conflicted)
	require.NotEmpty(t, diagnostics)
}

func TestCurrentUsageMalformedCorrectionDoesNotResurrectOldCost(t *testing.T) {
	cfg := parse.Config{SourceID: "source"}
	file := usageTestFile(t)
	first := parse.ParseUsage(cfg, file)
	file.BlobID, file.Body = "unreadable", ""
	file.CollectedAt = file.CollectedAt.Add(time.Hour)
	malformed := parse.ParseUsage(cfg, file)
	runs, _ := currentUsageRuns([]parse.UsageReport{malformed, first})
	require.Len(t, runs, 1)
	require.Equal(t, malformed.ID, runs[0].Report.ID)
	require.Nil(t, runs[0].Report.Totals.CostUSD)
}

func TestExtractUsageContinuesAndWritesValidSiblings(t *testing.T) {
	db, tx := mockdal.NewDal(t), mockdal.NewTransaction(t)
	ctx, logger := mockplugin.NewSubTaskContext(t), mocklog.NewLogger(t)
	cfg := &models.AisdlcScopeConfig{TenantId: "tenant", SourceId: "source", StationId: "station", UsageMode: "summary"}
	data := &AisdlcTaskData{Connection: &models.AisdlcConnection{}, Options: &AisdlcOptions{FullName: "group/results", ScopeConfig: cfg}}
	data.Connection.ID = 7
	ctx.On("GetData").Return(data)
	ctx.On("GetDal").Return(db)
	ctx.On("GetLogger").Return(logger)
	file := usageTestFile(t)
	db.On("All", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		*args[0].(*[]models.AisdlcRawFile) = []models.AisdlcRawFile{
			{FilePath: file.Path, Body: strings.Replace(file.Body, `"cost_usd": 2.925981`, `"cost_usd": NaN`, 1), BlobId: "partial", CollectedAt: file.CollectedAt},
			{FilePath: "bad/telemetry/usage.json", Body: "{broken", BlobId: "broken", CollectedAt: file.CollectedAt},
			{FilePath: "valid/telemetry/usage.json", Body: file.Body, BlobId: "valid", CollectedAt: file.CollectedAt},
		}
	}).Return(nil)
	logger.On("Error", nil, mock.Anything, mock.Anything).Return()
	db.On("Begin").Return(tx)
	var saved []models.AisdlcUsageReport
	tx.On("CreateOrUpdate", mock.Anything).Run(func(args mock.Arguments) {
		saved = append(saved, *args[0].(*models.AisdlcUsageReport))
	}).Return(nil)
	tx.On("Delete", mock.Anything, mock.Anything).Return(nil)
	tx.On("Create", mock.Anything).Return(nil)
	tx.On("Commit").Return(nil).Once()
	tx.On("Rollback").Return(nil).Once()
	db.On("First", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		*args[0].(*models.AisdlcCheckpoint) = models.AisdlcCheckpoint{Id: "checkpoint", ErrorMessage: "download error"}
	}).Return(nil)
	db.On("Update", mock.MatchedBy(func(row *models.AisdlcCheckpoint) bool {
		return strings.Contains(row.ErrorMessage, "download error") && strings.Contains(row.ErrorMessage, "non-finite")
	})).Return(nil)
	require.Nil(t, ExtractUsage(ctx))
	require.Len(t, saved, 3)
	require.Nil(t, saved[0].CostUSD)
	require.EqualValues(t, 505, *saved[0].InputTokens)
	require.Equal(t, "invalid", saved[1].ValidationState)
	require.Equal(t, "2.925981", *saved[2].CostUSD)
	for _, report := range saved {
		require.Equal(t, scopeRawParams(7, "group/results"), report.RawDataParams)
	}
}

func TestUsageDatabaseErrorsAreNotSwallowed(t *testing.T) {
	db, tx := mockdal.NewDal(t), mockdal.NewTransaction(t)
	db.On("Begin").Return(tx)
	writeErr := errors.Default.New("database unavailable")
	tx.On("CreateOrUpdate", mock.Anything).Return(writeErr).Once()
	tx.On("Rollback").Return(nil).Once()
	file := usageTestFile(t)
	report := parse.ParseUsage(parse.Config{SourceID: "source"}, file)
	err := saveUsage(db, 1, "group/results", &models.AisdlcScopeConfig{}, []parse.UsageReport{report}, nil)
	require.Equal(t, writeErr, err)
	tx.AssertNotCalled(t, "Commit")
}
