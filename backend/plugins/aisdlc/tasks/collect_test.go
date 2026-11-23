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
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/apache/devlake/core/errors"
	mockdal "github.com/apache/devlake/mocks/core/dal"
	mocklog "github.com/apache/devlake/mocks/core/log"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCollectUsageContinuesAfterMissingArtifact(t *testing.T) {
	file := usageTestFile(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/repository/commits/"):
			_, _ = w.Write([]byte(`{"id":"commit"}`))
		case strings.Contains(r.URL.Path, "/repository/tree"):
			_, _ = w.Write([]byte(`[
				{"id":"missing","path":"bad/telemetry/usage.json","type":"blob","mode":"100644"},
				{"id":"valid","path":"good/telemetry/usage.json","type":"blob","mode":"100644"}
			]`))
		case strings.Contains(r.URL.Path, "bad/"):
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte(file.Body))
		}
	}))
	defer server.Close()
	db, ctx, logger := mockdal.NewDal(t), mockplugin.NewSubTaskContext(t), mocklog.NewLogger(t)
	data := &AisdlcTaskData{
		Connection: &models.AisdlcConnection{GitlabConnectionId: 1},
		Options: &AisdlcOptions{FullName: "group/results", ScopeConfig: &models.AisdlcScopeConfig{
			UsageMode: "summary", Branch: "main",
		}},
	}
	data.Connection.ID = 7
	ctx.On("GetContext").Return(context.Background())
	ctx.On("GetDal").Return(db)
	ctx.On("GetLogger").Return(logger)
	ctx.On("GetData").Return(data)
	db.On("First", mock.AnythingOfType("*tasks.GitlabConnLookup"), mock.Anything).Run(func(args mock.Arguments) {
		*args[0].(*GitlabConnLookup) = GitlabConnLookup{Endpoint: server.URL, Token: "test"}
	}).Return(nil)
	notFound := errors.NotFound.New("not found")
	db.On("First", mock.AnythingOfType("*models.AisdlcRawFile"), mock.Anything).Return(notFound)
	db.On("IsErrorNotFound", notFound).Return(true)
	logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Once()
	var raws []models.AisdlcRawFile
	db.On("CreateOrUpdate", mock.AnythingOfType("*models.AisdlcRawFile")).Run(func(args mock.Arguments) {
		raws = append(raws, *args[0].(*models.AisdlcRawFile))
	}).Return(nil)
	db.On("CreateOrUpdate", mock.MatchedBy(func(row *models.AisdlcCheckpoint) bool {
		return row.FileCount == 2 && row.DownloadFailures == 1 &&
			row.SummaryFiles == 2 && row.SummaryFailures == 1 &&
			strings.Contains(row.ErrorMessage, "404") && row.ExaminedRevision == "commit"
	})).Return(nil).Once()
	require.Nil(t, CollectFiles(ctx))
	require.Len(t, raws, 2)
	require.Empty(t, raws[0].Body)
	require.Equal(t, "missing", raws[0].BlobId)
	require.Equal(t, file.Body, raws[1].Body)
	require.Equal(t, scopeRawParams(7, "group/results"), raws[1].RawDataParams)
}

func TestCollectAllUsageDownloadsFail(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/user":
					w.WriteHeader(http.StatusOK)
				case strings.Contains(r.URL.Path, "/repository/commits/"):
					_, _ = w.Write([]byte(`{"id":"commit"}`))
				case strings.Contains(r.URL.Path, "/repository/tree"):
					_, _ = w.Write([]byte(`[
						{"id":"first","path":"one/telemetry/usage.json","type":"blob","mode":"100644"},
						{"id":"second","path":"two/telemetry/usage.json","type":"blob","mode":"100644"}
					]`))
				default:
					w.WriteHeader(status)
				}
			}))
			defer server.Close()
			db, ctx, logger := mockdal.NewDal(t), mockplugin.NewSubTaskContext(t), mocklog.NewLogger(t)
			data := &AisdlcTaskData{
				Connection: &models.AisdlcConnection{GitlabConnectionId: 1},
				Options: &AisdlcOptions{FullName: "group/results", ScopeConfig: &models.AisdlcScopeConfig{
					UsageMode: "summary", Branch: "main",
				}},
			}
			data.Connection.ID = 7
			ctx.On("GetContext").Return(context.Background())
			ctx.On("GetDal").Return(db)
			ctx.On("GetLogger").Return(logger)
			ctx.On("GetData").Return(data)
			db.On("First", mock.AnythingOfType("*tasks.GitlabConnLookup"), mock.Anything).Run(func(args mock.Arguments) {
				*args[0].(*GitlabConnLookup) = GitlabConnLookup{Endpoint: server.URL, Token: "test"}
			}).Return(nil)
			notFound := errors.NotFound.New("not found")
			db.On("First", mock.AnythingOfType("*models.AisdlcRawFile"), mock.Anything).Return(notFound)
			db.On("IsErrorNotFound", notFound).Return(true)
			logger.On("Error", mock.Anything, mock.Anything, mock.Anything).Twice()
			db.On("CreateOrUpdate", mock.MatchedBy(func(row *models.AisdlcRawFile) bool {
				return row.Body == ""
			})).Return(nil).Twice()
			db.On("CreateOrUpdate", mock.MatchedBy(func(row *models.AisdlcCheckpoint) bool {
				return row.FileCount == 2 && row.DownloadFailures == 2 &&
					row.SummaryFiles == 2 && row.SummaryFailures == 2 &&
					strings.Contains(row.ErrorMessage, strconv.Itoa(status))
			})).Return(nil).Once()
			require.Nil(t, CollectFiles(ctx))
		})
	}
}
