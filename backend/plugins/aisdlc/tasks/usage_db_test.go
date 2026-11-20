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
	"sync"
	"testing"
	"time"

	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/impls/dalgorm"
	"github.com/apache/devlake/plugins/aisdlc/models"
	"github.com/apache/devlake/plugins/aisdlc/models/migrationscripts"
	"github.com/apache/devlake/plugins/aisdlc/tasks/parse"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type usageMigrationResources struct {
	context.BasicRes
	db dal.Dal
}

func (r usageMigrationResources) GetDal() dal.Dal { return r.db }

// These opt-in tests require disposable databases. They exercise real migrations,
// decimal/null persistence, replacement of current reports, and scope cleanup.
func TestUsageDatabaseLifecycle(t *testing.T) {
	for _, dialect := range []string{"mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv("AISDLC_TEST_" + strings.ToUpper(dialect) + "_DSN")
			if dsn == "" {
				t.Skip("set AISDLC_TEST_" + strings.ToUpper(dialect) + "_DSN to a disposable database")
			}
			var driver gorm.Dialector = mysql.Open(dsn)
			if dialect == "postgres" {
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			d := dalgorm.NewDalgorm(db)
			res := usageMigrationResources{db: d}
			for _, migration := range migrationscripts.All() {
				require.Nil(t, migration.Up(res), migration.Name())
			}
			migrations := migrationscripts.All()
			require.Nil(t, migrations[len(migrations)-1].Up(res), "migration replay")
			tables := []dal.Tabler{&models.AisdlcUsageReport{}, &models.AisdlcUsageModel{}, &models.AisdlcUsageRun{}}
			for _, table := range tables {
				shape, err := schema.Parse(table, &sync.Map{}, schema.NamingStrategy{})
				require.NoError(t, err)
				for _, column := range shape.DBNames {
					require.True(t, db.Migrator().HasColumn(table, column), "%s.%s", table.TableName(), column)
				}
			}
			require.True(t, db.Migrator().HasColumn(&models.AisdlcScopeConfig{}, "usage_mode"))
			// Use a dedicated connection identifier and clean only these test rows.
			const connectionID = uint64(987654)
			for _, table := range tables {
				require.Nil(t, d.Delete(table, dal.Where("connection_id = ?", connectionID)))
			}
			cfg := &models.AisdlcScopeConfig{TenantId: "tenant", SourceId: "source", StationId: "rfe", UsageMode: "summary"}
			config := parse.Config{TenantID: cfg.TenantId, SourceID: cfg.SourceId, StationID: cfg.StationId, ProjectPath: "group/results"}
			file := usageTestFile(t)
			report := parse.ParseUsage(config, file)
			reports := []parse.UsageReport{report}
			runs, _ := currentUsageRuns(reports)
			require.Nil(t, saveUsage(d, connectionID, config.ProjectPath, cfg, reports, runs))
			require.Nil(t, saveUsage(d, connectionID, config.ProjectPath, cfg, reports, runs), "same blob replay")
			var saved models.AisdlcUsageReport
			require.Nil(t, d.First(&saved, dal.Where("id = ? AND connection_id = ?", report.ID, connectionID)))
			require.Equal(t, "2.925981", *saved.CostUSD)
			require.Nil(t, saved.ReasoningTokens)
			require.Nil(t, saved.Turns)
			require.EqualValues(t, 333152, *saved.DurationMs)
			require.Equal(t, scopeRawParams(connectionID, config.ProjectPath), saved.RawDataParams)

			file.BlobID = "corrected"
			file.CollectedAt = file.CollectedAt.Add(time.Hour)
			file.Body = strings.ReplaceAll(file.Body, "2.925981", "3.000001")
			corrected := parse.ParseUsage(config, file)
			reports = append(reports, corrected)
			runs, _ = currentUsageRuns(reports)
			require.Nil(t, saveUsage(d, connectionID, config.ProjectPath, cfg, reports, runs))
			var pointers []models.AisdlcUsageRun
			require.Nil(t, d.All(&pointers, dal.Where("connection_id = ?", connectionID)))
			require.Len(t, pointers, 1)
			require.Equal(t, corrected.ID, pointers[0].ReportId)
			var history []models.AisdlcUsageReport
			require.Nil(t, d.All(&history, dal.Where("connection_id = ?", connectionID)))
			require.Len(t, history, 2)

			// Re-extraction must overwrite old values with NULL, not leave a stale
			// cost behind. Removed model children must disappear in the same tx.
			corrected.Totals.CostUSD = nil
			corrected.Models = nil
			reports[1] = corrected
			runs, _ = currentUsageRuns(reports)
			require.Nil(t, saveUsage(d, connectionID, config.ProjectPath, cfg, reports, runs))
			saved = models.AisdlcUsageReport{}
			require.Nil(t, d.First(&saved, dal.Where("id = ? AND connection_id = ?", corrected.ID, connectionID)))
			require.Nil(t, saved.CostUSD)
			var breakdown []models.AisdlcUsageModel
			require.Nil(t, d.All(&breakdown, dal.Where("connection_id = ? AND report_id = ?", connectionID, corrected.ID)))
			require.Empty(t, breakdown)

			// A second scope on the same connection must survive scope cleanup.
			other := config
			other.ProjectPath = "group/other"
			otherReport := parse.ParseUsage(other, file)
			otherReports := []parse.UsageReport{otherReport}
			otherRuns, _ := currentUsageRuns(otherReports)
			require.Nil(t, saveUsage(d, connectionID, other.ProjectPath, cfg, otherReports, otherRuns))
			for _, table := range tables {
				column := dal.QuoteIdent(d.Dialect(), "_raw_data_params")
				require.Nil(t, d.Delete(table, dal.Where(column+" = ?", scopeRawParams(connectionID, config.ProjectPath))))
			}
			require.Nil(t, d.All(&pointers, dal.Where("connection_id = ?", connectionID)))
			require.Len(t, pointers, 1)
			require.Equal(t, otherReport.ID, pointers[0].ReportId)
			for _, table := range tables {
				require.Nil(t, d.Delete(table, dal.Where("connection_id = ?", connectionID)))
			}
		})
	}
}
