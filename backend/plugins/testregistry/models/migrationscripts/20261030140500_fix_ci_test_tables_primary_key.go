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

var _ plugin.MigrationScript = (*fixCITestTablesPrimaryKey)(nil)

type fixCITestTablesPrimaryKey struct{}

type ciTestPrimaryKey struct {
	table   string
	columns string
}

func (*fixCITestTablesPrimaryKey) Up(basicRes context.BasicRes) errors.Error {
	db := basicRes.GetDal()
	logger := basicRes.GetLogger()
	targets := []ciTestPrimaryKey{
		{table: "ci_test_jobs", columns: "connection_id, job_id"},
		{table: "ci_test_suites", columns: "connection_id, job_id, suite_id"},
		{table: "ci_test_cases", columns: "connection_id, job_id, suite_id, test_case_id"},
		{table: "ci_tekton_tasks", columns: "connection_id, job_id, task_name"},
	}
	for _, target := range targets {
		if !db.HasTable(target.table) {
			continue
		}
		hasPK, err := tableHasPrimaryKey(db, target.table)
		if err != nil {
			return err
		}
		if hasPK {
			continue
		}
		sql := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s)", target.table, target.columns)
		if execErr := db.Exec(sql); execErr != nil {
			return errors.Default.Wrap(execErr, "failed: "+sql)
		}
		logger.Info("added composite primary key on %s", target.table)
	}
	return nil
}

func tableHasPrimaryKey(db dal.Dal, table string) (bool, errors.Error) {
	schemaExpr := "DATABASE()"
	if db.Dialect() == "postgres" {
		schemaExpr = "current_schema()"
	}
	cursor, err := db.Cursor(
		dal.From("information_schema.table_constraints"),
		dal.Where(
			fmt.Sprintf("table_schema = %s AND table_name = ? AND constraint_type = 'PRIMARY KEY'", schemaExpr),
			table,
		),
	)
	if err != nil {
		return false, err
	}
	defer cursor.Close()
	if cursor.Next() {
		return true, nil
	}
	return false, nil
}

func (*fixCITestTablesPrimaryKey) Version() uint64 {
	return 20261030140500
}

func (*fixCITestTablesPrimaryKey) Name() string {
	return "testregistry add composite primary keys on ci test tables when missing"
}
