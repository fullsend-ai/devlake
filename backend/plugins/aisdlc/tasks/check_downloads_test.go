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
	"strings"
	"testing"
	"unicode/utf8"

	mockdal "github.com/apache/devlake/mocks/core/dal"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCheckDownloads(t *testing.T) {
	for _, tt := range []struct {
		name       string
		checkpoint models.AisdlcCheckpoint
		wantError  string
	}{
		{"all files 403", models.AisdlcCheckpoint{FileCount: 2, DownloadFailures: 2}, "all 2 AISDLC artifact downloads failed"},
		{"all summaries 429 with valid results", models.AisdlcCheckpoint{
			FileCount: 3, DownloadFailures: 2, SummaryFiles: 2, SummaryFailures: 2,
		}, "all 2 AISDLC usage summary downloads failed"},
		{"all results with valid summaries", models.AisdlcCheckpoint{
			FileCount: 3, DownloadFailures: 2, SummaryFiles: 1,
		}, "all 2 AISDLC result downloads failed"},
		{"mixed files", models.AisdlcCheckpoint{
			FileCount: 4, DownloadFailures: 1, SummaryFiles: 2, SummaryFailures: 1,
		}, ""},
		{"no files", models.AisdlcCheckpoint{}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, ctx, task := mockdal.NewDal(t), mockplugin.NewSubTaskContext(t), mockplugin.NewTaskContext(t)
			ctx.On("TaskContext").Return(task)
			task.On("SyncPolicy").Return(nil)
			ctx.On("GetData").Return(&AisdlcTaskData{
				Connection: &models.AisdlcConnection{},
				Options:    &AisdlcOptions{ScopeConfig: &models.AisdlcScopeConfig{UsageMode: "summary"}},
			})
			ctx.On("GetDal").Return(db)
			db.On("First", mock.AnythingOfType("*models.AisdlcCheckpoint"), mock.Anything).Run(func(args mock.Arguments) {
				*args[0].(*models.AisdlcCheckpoint) = tt.checkpoint
			}).Return(nil)
			err := CheckDownloads(ctx)
			if tt.wantError == "" {
				require.Nil(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantError)
			}
		})
	}
}

func TestCheckpointDiagnosticsBounded(t *testing.T) {
	messages := make([]string, 1000)
	for i := range messages {
		messages[i] = strings.Repeat("é", 3000)
	}
	summary := checkpointDiagnostics(messages)
	require.Contains(t, summary, "1000 diagnostic(s)")
	require.LessOrEqual(t, len(summary), checkpointMessageBytes)
	require.True(t, utf8.ValidString(summary))
	require.LessOrEqual(t, len(truncateCheckpoint(summary+"\n"+summary)), checkpointMessageBytes)
}

func TestAppendCheckpointErrorBoundsExistingMessage(t *testing.T) {
	db := mockdal.NewDal(t)
	db.On("First", mock.AnythingOfType("*models.AisdlcCheckpoint"), mock.Anything).Run(func(args mock.Arguments) {
		*args[0].(*models.AisdlcCheckpoint) = models.AisdlcCheckpoint{
			ErrorMessage: strings.Repeat("é", checkpointMessageBytes),
		}
	}).Return(nil)
	db.On("Update", mock.MatchedBy(func(row *models.AisdlcCheckpoint) bool {
		return len(row.ErrorMessage) <= checkpointMessageBytes && utf8.ValidString(row.ErrorMessage) &&
			strings.Contains(row.ErrorMessage, "truncated")
	})).Return(nil)
	require.Nil(t, appendCheckpointError(db, 7, "group/results", "another diagnostic"))
}
