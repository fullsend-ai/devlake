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
	"fmt"

	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
)

var _ plugin.MigrationScript = (*fixAiReviewDomainColumns)(nil)

type fixAiReviewDomainColumns struct{}

func (*fixAiReviewDomainColumns) Up(basicRes context.BasicRes) errors.Error {
	db := basicRes.GetDal()
	// AutoMigrate with embedded structs does not reliably add columns whose names
	// start with '_'. Use explicit ALTER TABLE statements instead.
	dialect := db.Dialect()
	idType := "bigint unsigned"
	remarkType := "longtext"
	if dialect == "postgres" {
		idType = "bigint"
		remarkType = "text"
	}
	tables := []string{"ai_reviews", "ai_failure_predictions", "ai_prediction_metrics"}
	columns := []struct {
		name string
		typ  string
	}{
		{"_raw_data_params", "varchar(255)"},
		{"_raw_data_table", "varchar(255)"},
		{"_raw_data_id", idType},
		{"_raw_data_remark", remarkType},
	}
	for _, table := range tables {
		for _, column := range columns {
			sql := fmt.Sprintf(
				"ALTER TABLE %s ADD COLUMN %s %s",
				dal.QuoteIdent(dialect, table),
				dal.QuoteIdent(dialect, column.name),
				column.typ,
			)
			if err := db.Exec(sql); err != nil {
				return errors.Default.Wrap(err, "failed: "+sql)
			}
		}
	}
	return nil
}

func (*fixAiReviewDomainColumns) Version() uint64 {
	return 20260422000002
}

func (*fixAiReviewDomainColumns) Name() string {
	return "add missing _raw_data_* columns to ai review domain tables"
}
